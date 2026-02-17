package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Bitcask struct {
	fileMu        sync.RWMutex
	keyDir        *KeyDir
	activeFile    *DataFile
	readOnlyFiles []*DataFile
	config        Config
	lockFile      *LockFile
	syncCancel    context.CancelFunc
	syncDone      chan struct{}
}

func NewBitcask(config Config) (*Bitcask, error) {
	if err := os.MkdirAll(config.Directory, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Acquire advisory lock
	lock, err := AcquireLock(config.Directory)
	if err != nil {
		return nil, err
	}

	fileIds, err := getDataFileIds(config.Directory)
	if err != nil {
		lock.Release()
		return nil, fmt.Errorf("failed to scan data files: %w", err)
	}

	b := &Bitcask{
		keyDir:        NewKeyDir(),
		readOnlyFiles: make([]*DataFile, 0),
		config:        config,
		lockFile:      lock,
	}

	if len(fileIds) == 0 {
		activeFile, err := NewDataFile(config.Directory, 0)
		if err != nil {
			lock.Release()
			return nil, fmt.Errorf("failed to create active file: %w", err)
		}
		b.activeFile = activeFile
		b.startSyncWorker()
		return b, nil
	}

	slices.Sort(fileIds)

	// Open all existing files as read-only
	for _, id := range fileIds {
		df, err := OpenDataFile(config.Directory, id)
		if err != nil {
			lock.Release()
			return nil, fmt.Errorf("failed to open data file %d: %w", id, err)
		}
		b.readOnlyFiles = append(b.readOnlyFiles, df)
	}

	// Startup recovery: rebuild keydir from hint files and data files
	if err := b.rebuildKeyDir(); err != nil {
		lock.Release()
		return nil, fmt.Errorf("failed to rebuild keydir: %w", err)
	}

	nextId := fileIds[len(fileIds)-1] + 1
	activeFile, err := NewDataFile(config.Directory, nextId)
	if err != nil {
		lock.Release()
		return nil, fmt.Errorf("failed to create active file: %w", err)
	}
	b.activeFile = activeFile

	b.startSyncWorker()
	return b, nil
}

// rebuildKeyDir rebuilds the in-memory key directory from hint files and data files.
// Files with hint files use the fast path; others are scanned record by record.
func (b *Bitcask) rebuildKeyDir() error {
	var hintFiles []*HintFile
	var dataFilesNoHint []*DataFile

	for _, df := range b.readOnlyFiles {
		hf, err := OpenHintFile(b.config.Directory, df.fileId)
		if err == nil {
			hintFiles = append(hintFiles, hf)
		} else {
			dataFilesNoHint = append(dataFilesNoHint, df)
		}
	}

	// Rebuild from hint files first (fast path)
	if len(hintFiles) > 0 {
		b.keyDir.RebuildFromHintFiles(hintFiles)
		for _, hf := range hintFiles {
			hf.Close()
		}
	}

	// Rebuild from data files without hints (slow path)
	if len(dataFilesNoHint) > 0 {
		b.keyDir.RebuildFromFiles(dataFilesNoHint)
	}

	return nil
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
	bc.fileMu.Lock()
	defer bc.fileMu.Unlock()

	r := NewRecord([]byte(key), []byte(value))
	offset, err := bc.activeFile.AppendRecord(r)
	if err != nil {
		return false, fmt.Errorf("append error: %w", err)
	}

	rmd := &RecordMetadata{
		fileId:    bc.activeFile.fileId,
		offset:    offset,
		size:      uint32(r.RecordSize()),
		timestamp: r.timestamp,
	}

	bc.keyDir.Update(key, rmd)

	// Sync if strategy is "always"
	if bc.config.SyncStrategy == SyncAlways {
		bc.activeFile.file.Sync()
	}

	// File rotation check
	if bc.activeFile.offset >= bc.config.MaxFileSize {
		if err := bc.rotateActiveFile(); err != nil {
			return false, fmt.Errorf("file rotation error: %w", err)
		}
	}

	return true, nil
}

// rotateActiveFile converts the current active file to read-only and creates a new one.
// Caller must hold fileMu.Lock.
func (bc *Bitcask) rotateActiveFile() error {
	bc.activeFile.file.Sync()
	bc.activeFile.isReadOnly = true
	bc.readOnlyFiles = append(bc.readOnlyFiles, bc.activeFile)

	nextId := bc.activeFile.fileId + 1
	newFile, err := NewDataFile(bc.config.Directory, nextId)
	if err != nil {
		return err
	}
	bc.activeFile = newFile
	return nil
}

func (bc *Bitcask) Get(key string) (string, error) {
	rmd := bc.keyDir.Get(key)
	if rmd == nil {
		return "", fmt.Errorf("key not found")
	}

	bc.fileMu.RLock()
	var df *DataFile
	var fileSize int64
	if rmd.fileId == bc.activeFile.fileId {
		df = bc.activeFile
		fileSize = bc.activeFile.offset // snapshot offset under lock
	} else {
		for _, f := range bc.readOnlyFiles {
			if f.fileId == rmd.fileId {
				df = f
				fileSize = f.offset
				break
			}
		}
	}
	bc.fileMu.RUnlock()

	if df == nil {
		return "", fmt.Errorf("data file %d not found", rmd.fileId)
	}

	rec, err := df.ReadRecordAt(rmd.offset, fileSize)
	if err != nil {
		return "", fmt.Errorf("failed to read record: %w", err)
	}

	// CRC validation
	if !ValidateCrc(rec) {
		return "", fmt.Errorf("CRC mismatch for key %q", key)
	}

	return string(rec.value), nil
}

func (bc *Bitcask) Delete(key string) (bool, error) {
	bc.fileMu.Lock()
	defer bc.fileMu.Unlock()

	rmd := bc.keyDir.Get(key)
	if rmd == nil {
		return false, fmt.Errorf("key not found")
	}

	r := NewTombstoneRecord([]byte(key))
	if _, err := bc.activeFile.AppendRecord(r); err != nil {
		return false, fmt.Errorf("failed to write tombstone: %w", err)
	}

	bc.keyDir.Remove(key)

	if bc.config.SyncStrategy == SyncAlways {
		bc.activeFile.file.Sync()
	}

	return true, nil
}

func (bc *Bitcask) Sync() error {
	bc.fileMu.Lock()
	defer bc.fileMu.Unlock()

	return bc.activeFile.file.Sync()
}

func (bc *Bitcask) Close() error {
	// Stop sync worker
	if bc.syncCancel != nil {
		bc.syncCancel()
		<-bc.syncDone
	}

	bc.fileMu.Lock()
	defer bc.fileMu.Unlock()

	if err := bc.activeFile.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync active file: %w", err)
	}

	if err := bc.activeFile.Close(); err != nil {
		return fmt.Errorf("failed to close active file: %w", err)
	}

	for _, df := range bc.readOnlyFiles {
		if err := df.Close(); err != nil {
			return fmt.Errorf("failed to close data file %d: %w", df.fileId, err)
		}
	}

	// Release lock
	if bc.lockFile != nil {
		bc.lockFile.Release()
	}

	return nil
}

func (bc *Bitcask) Merge() error {
	// Freeze current active file and snapshot stale files
	bc.fileMu.Lock()
	if len(bc.readOnlyFiles) == 0 {
		bc.fileMu.Unlock()
		return nil
	}

	// Rotate active file so its data can be merged
	staleFiles := make([]*DataFile, len(bc.readOnlyFiles))
	copy(staleFiles, bc.readOnlyFiles)
	bc.fileMu.Unlock()

	compactor := NewCompactor(bc.config.Directory, bc.keyDir)

	mergedFiles, err := compactor.MergeStaleFiles(staleFiles)
	if err != nil {
		return fmt.Errorf("failed to merge stale files: %w", err)
	}

	if err := compactor.removeOldFiles(); err != nil {
		return fmt.Errorf("failed to remove old files: %w", err)
	}

	bc.fileMu.Lock()
	if mergedFiles != nil {
		bc.readOnlyFiles = mergedFiles
	} else {
		bc.readOnlyFiles = make([]*DataFile, 0)
	}
	bc.fileMu.Unlock()

	return nil
}

func (bc *Bitcask) ListKeys() []string {
	return bc.keyDir.Keys()
}

func (bc *Bitcask) Fold(fn func(key string, value string) error) error {
	keys := bc.keyDir.Keys()
	for _, key := range keys {
		value, err := bc.Get(key)
		if err != nil {
			continue // key may have been deleted concurrently
		}
		if err := fn(key, value); err != nil {
			return err
		}
	}
	return nil
}

// startSyncWorker starts a background goroutine for interval-based syncing.
func (bc *Bitcask) startSyncWorker() {
	if bc.config.SyncStrategy != SyncInterval || bc.config.SyncInterval <= 0 {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	bc.syncCancel = cancel
	bc.syncDone = make(chan struct{})

	go func() {
		defer close(bc.syncDone)
		ticker := time.NewTicker(time.Duration(bc.config.SyncInterval) * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				bc.Sync()
			}
		}
	}()
}
