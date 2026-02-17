package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Compactor struct {
	directory string
	keyDir    *KeyDir
	oldFiles  []*DataFile
}

func NewCompactor(directory string, keyDir *KeyDir) *Compactor {
	return &Compactor{
		directory: directory,
		keyDir:    keyDir,
	}
}

// MergeStaleFiles reads all live records from the given stale data files,
// writes them into a new merged data file, creates a corresponding hint file,
// and returns the new merged data files. The caller should replace its
// readOnlyFiles with the returned files and call removeOldFiles to clean up.
func (c *Compactor) MergeStaleFiles(dfs []*DataFile) ([]*DataFile, error) {
	if len(dfs) == 0 {
		return nil, nil
	}

	c.oldFiles = dfs

	// Determine the next available file ID
	existingIds, err := getDataFileIds(c.directory)
	if err != nil {
		return nil, fmt.Errorf("failed to scan data files: %w", err)
	}
	var maxId uint32
	for _, id := range existingIds {
		if id > maxId {
			maxId = id
		}
	}
	mergeFileId := maxId + 1

	// Create a new data file for merged records
	mergedFile, err := NewDataFile(c.directory, mergeFileId)
	if err != nil {
		return nil, fmt.Errorf("failed to create merged file: %w", err)
	}

	// Copy live records from each stale file into the merged file
	for _, df := range dfs {
		if err := c.mergeFile(df, mergedFile); err != nil {
			mergedFile.Close()
			return nil, fmt.Errorf("failed to merge file %d: %w", df.fileId, err)
		}
	}

	// If no live records were found, clean up the empty file
	if mergedFile.offset == 0 {
		mergedFile.Close()
		os.Remove(filepath.Join(c.directory, fmt.Sprintf("%d.data", mergeFileId)))
		return nil, nil
	}

	// Create a hint file for fast KeyDir rebuilds
	if err := c.createHintFile(mergedFile); err != nil {
		mergedFile.Close()
		return nil, fmt.Errorf("failed to create hint file: %w", err)
	}

	mergedFile.Close()

	// Reopen as read-only
	readOnlyMerged, err := OpenDataFile(c.directory, mergeFileId)
	if err != nil {
		return nil, fmt.Errorf("failed to reopen merged file: %w", err)
	}

	return []*DataFile{readOnlyMerged}, nil
}

// mergeFile reads all records from a stale data file and writes live records
// to the merged file, updating the KeyDir to point to the new locations.
func (c *Compactor) mergeFile(staleFile *DataFile, mergedFile *DataFile) error {
	var offset int64

	for offset < staleFile.offset {
		rec, err := staleFile.ReadRecord(offset)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read record at offset %d: %w", offset, err)
		}

		recordSize := int64(24 + rec.keySize + rec.valueSize)
		key := string(rec.key)

		// A record is live if the KeyDir entry for its key points to this
		// exact location in this file
		meta := c.keyDir.Get(key)
		if meta != nil && meta.fileId == staleFile.fileId {
			expectedValuePos := offset + 24 + int64(rec.keySize)
			if meta.valuePosition == expectedValuePos {
				// Live record — copy to merged file
				writeOffset := mergedFile.offset
				if _, err := mergedFile.AppendRecord(rec); err != nil {
					return fmt.Errorf("failed to write merged record: %w", err)
				}

				// Update KeyDir to point to the new location
				c.keyDir.Update(key, &RecordMetadata{
					fileId:        mergedFile.fileId,
					valueSize:     rec.valueSize,
					valuePosition: writeOffset + 24 + int64(rec.keySize),
					timestamp:     rec.timestamp,
				})
			}
		}

		offset += recordSize
	}

	return nil
}

// createHintFile creates a hint file for the given data file. The hint file
// stores key locations for fast KeyDir rebuilds without scanning full records.
func (c *Compactor) createHintFile(df *DataFile) error {
	hintPath := filepath.Join(c.directory, fmt.Sprintf("%d.hint", df.fileId))
	f, err := os.OpenFile(hintPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create hint file: %w", err)
	}

	hf := &HintFile{
		fileId: df.fileId,
		file:   f,
		offset: 0,
	}
	defer hf.Close()

	var offset int64
	for offset < df.offset {
		rec, err := df.ReadRecord(offset)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read record at offset %d: %w", offset, err)
		}

		entry := &HintEntry{
			timestamp: rec.timestamp,
			keySize:   rec.keySize,
			valueSize: rec.valueSize,
			offset:    offset,
			key:       rec.key,
		}

		if err := hf.WriteEntry(entry); err != nil {
			return fmt.Errorf("failed to write hint entry: %w", err)
		}

		offset += int64(24 + rec.keySize + rec.valueSize)
	}

	return nil
}

// removeOldFiles closes and deletes the stale data files that were merged.
func (c *Compactor) removeOldFiles() error {
	for _, df := range c.oldFiles {
		path := filepath.Join(c.directory, fmt.Sprintf("%d.data", df.fileId))
		df.Close()
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("failed to remove old data file %s: %w", path, err)
		}
		// Also remove any existing hint file for the old data file
		hintPath := filepath.Join(c.directory, fmt.Sprintf("%d.hint", df.fileId))
		os.Remove(hintPath) // ignore error, hint file may not exist
	}
	c.oldFiles = nil
	return nil
}
