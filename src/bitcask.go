package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type Bitcask struct {
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
