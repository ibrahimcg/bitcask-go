package main

import "io"

type KeyDir struct {
	index map[string]*RecordMetadata
}

func NewKeyDir() *KeyDir {
	return &KeyDir{
		index: make(map[string]*RecordMetadata),
	}
}

func (kd *KeyDir) Update(key string, metadata *RecordMetadata) {
	kd.index[key] = metadata
}

func (kd *KeyDir) Get(key string) *RecordMetadata {
	val, ok := kd.index[key]

	if ok {
		return val
	}
	return nil
}

func (kd *KeyDir) Remove(key string) {
	delete(kd.index, key)
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
				break
			}

			key := string(rec.key)
			recordSize := int64(24 + rec.keySize + rec.valueSize)

			if rec.valueSize == 0 {
				kd.Remove(key)
			} else {
				kd.Update(key, &RecordMetadata{
					fileId:        df.fileId,
					valueSize:     rec.valueSize,
					valuePosition: offset + 24 + int64(rec.keySize),
					timestamp:     rec.timestamp,
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
			continue
		}

		for _, entry := range entries {
			key := string(entry.key)

			if entry.valueSize == 0 {
				kd.Remove(key)
			} else {
				kd.Update(key, &RecordMetadata{
					fileId:        hf.fileId,
					valueSize:     entry.valueSize,
					valuePosition: entry.offset + 24 + int64(entry.keySize),
					timestamp:     entry.timestamp,
				})
			}
		}
	}
}
