package main

import (
	"io"
	"log"
	"sync"
)

type KeyDir struct {
	mu    sync.RWMutex
	index map[string]*RecordMetadata
}

func NewKeyDir() *KeyDir {
	return &KeyDir{
		index: make(map[string]*RecordMetadata),
	}
}

func (kd *KeyDir) Update(key string, metadata *RecordMetadata) {
	kd.mu.Lock()
	kd.index[key] = metadata
	kd.mu.Unlock()
}

func (kd *KeyDir) Get(key string) *RecordMetadata {
	kd.mu.RLock()
	val := kd.index[key]
	kd.mu.RUnlock()
	return val
}

func (kd *KeyDir) Remove(key string) {
	kd.mu.Lock()
	delete(kd.index, key)
	kd.mu.Unlock()
}

func (kd *KeyDir) Len() int {
	kd.mu.RLock()
	n := len(kd.index)
	kd.mu.RUnlock()
	return n
}

func (kd *KeyDir) Keys() []string {
	kd.mu.RLock()
	keys := make([]string, 0, len(kd.index))
	for k := range kd.index {
		keys = append(keys, k)
	}
	kd.mu.RUnlock()
	return keys
}

func (kd *KeyDir) RebuildFromFiles(files []*DataFile) {
	for _, df := range files {
		var offset int64
		for offset < df.offset {
			rec, err := df.ReadRecord(offset)
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("warning: skipping corrupt record at offset %d in file %d: %v", offset, df.fileId, err)
				break
			}

			if !ValidateCrc(rec) {
				log.Printf("warning: CRC mismatch at offset %d in file %d, skipping remaining records", offset, df.fileId)
				break
			}

			key := string(rec.key)
			recordSize := rec.RecordSize()

			if rec.valueSize == TombstoneValue {
				kd.Remove(key)
			} else {
				kd.Update(key, &RecordMetadata{
					fileId:    df.fileId,
					offset:    offset,
					size:      uint32(recordSize),
					timestamp: rec.timestamp,
				})
			}

			offset += recordSize
		}
	}
}

func (kd *KeyDir) RebuildFromHintFiles(hintFiles []*HintFile) {
	for _, hf := range hintFiles {
		entries, err := hf.ReadEntries()
		if err != nil {
			log.Printf("warning: failed to read hint file %d: %v", hf.fileId, err)
			continue
		}

		for _, entry := range entries {
			key := string(entry.key)

			kd.Update(key, &RecordMetadata{
				fileId:    hf.fileId,
				offset:    entry.offset,
				size:      entry.recordSize,
				timestamp: entry.timestamp,
			})
		}
	}
}
