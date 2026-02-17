package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
)

type Bitcask struct {
	mu                 sync.RWMutex
	keyDir             *KeyDir
	activeFile         *DataFile
	readOnlyFiles      []*DataFile
	dataFilesDirectory string
}

func NewBitcask(config Config) (*Bitcask, error) {
	if err := os.MkdirAll(config.Directory, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	fileIds, err := getDataFileIds(config.Directory)
	if err != nil {
		return nil, fmt.Errorf("failed to scan data files: %w", err)
	}

	b := &Bitcask{
		keyDir:             NewKeyDir(),
		readOnlyFiles:      make([]*DataFile, 0),
		dataFilesDirectory: config.Directory,
	}

	if len(fileIds) == 0 {
		activeFile, err := NewDataFile(config.Directory, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to create active file: %w", err)
		}
		b.activeFile = activeFile
		return b, nil
	}

	slices.Sort(fileIds)

	for _, id := range fileIds {
		df, err := OpenDataFile(config.Directory, id)
		if err != nil {
			return nil, fmt.Errorf("failed to open data file %d: %w", id, err)
		}
		b.readOnlyFiles = append(b.readOnlyFiles, df)
	}

	nextId := fileIds[len(fileIds)-1] + 1
	activeFile, err := NewDataFile(config.Directory, nextId)
	if err != nil {
		return nil, fmt.Errorf("failed to create active file: %w", err)
	}
	b.activeFile = activeFile

	return b, nil
}

func getDataFileIds(dir string) ([]uint32, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.data"))
	if err != nil {
		return nil, err
	}

	var ids []uint32
	for _, match := range matches {
		name := strings.TrimSuffix(filepath.Base(match), ".data")
		id, err := strconv.ParseUint(name, 10, 32)
		if err != nil {
			continue
		}
		ids = append(ids, uint32(id))
	}
	return ids, nil
}

func (bc *Bitcask) Put(key string, value string) (bool, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	r := NewRecord([]byte(key), []byte(value))
	o := bc.activeFile.offset

	if _, err := bc.activeFile.AppendRecord(r); err != nil {
		return false, fmt.Errorf("Append Error")
	}

	rmd := &RecordMetadata{
		fileId:        bc.activeFile.fileId,
		valueSize:     r.valueSize,
		valuePosition: o + 24 + int64(len(key)),
		timestamp:     r.timestamp,
	}

	bc.keyDir.Update(key, rmd)

	return true, nil
}

func (bc *Bitcask) Get(key string) (string, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	rmd := bc.keyDir.Get(key)
	if rmd == nil {
		return "", fmt.Errorf("key not found")
	}

	recordOffset := rmd.valuePosition - 24 - int64(len(key))

	var rec *Record
	var err error

	if rmd.fileId == bc.activeFile.fileId {
		rec, err = bc.activeFile.ReadRecord(recordOffset)
	} else {
		for _, df := range bc.readOnlyFiles {
			if df.fileId == rmd.fileId {
				rec, err = df.ReadRecord(recordOffset)
				break
			}
		}
		if rec == nil && err == nil {
			return "", fmt.Errorf("data file %d not found", rmd.fileId)
		}
	}

	if err != nil {
		return "", fmt.Errorf("failed to read record: %w", err)
	}

	return string(rec.value), nil
}

func (bc *Bitcask) Delete(key string) (bool, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	rmd := bc.keyDir.Get(key)
	if rmd == nil {
		return false, fmt.Errorf("key not found")
	}

	// Append a tombstone record (empty value) to the active file
	r := NewRecord([]byte(key), []byte{})
	if _, err := bc.activeFile.AppendRecord(r); err != nil {
		return false, fmt.Errorf("failed to write tombstone: %w", err)
	}

	bc.keyDir.Remove(key)

	return true, nil
}

func (bc *Bitcask) Sync() error {
	bc.mu.Lock()
	defer bc.mu.Lock()

	return bc.activeFile.file.Sync()
}
