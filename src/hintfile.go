package main

import (
	"io"
	"os"
)

type HintFile struct {
	fileId uint32
	file   *os.File
	offset int64
}

func (hf *HintFile) WriteEntry(entry *HintEntry) error {
	data := entry.Encode()

	n, err := hf.file.WriteAt(data, hf.offset)
	if err != nil {
		return err
	}
	hf.offset += int64(n)

	return nil
}

func (hf *HintFile) ReadEntries() ([]*HintEntry, error) {
	reader := io.NewSectionReader(hf.file, 0, hf.offset)

	var entries []*HintEntry
	for {
		entry, err := DecodeHintEntry(reader)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (hf *HintFile) Close() error {
	return hf.file.Close()
}
