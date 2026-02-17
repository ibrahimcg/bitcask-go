package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type HintFile struct {
	fileId uint32
	file   *os.File
	offset int64
}

// OpenHintFile opens an existing hint file as read-only.
func OpenHintFile(dir string, fileId uint32) (*HintFile, error) {
	path := filepath.Join(dir, fmt.Sprintf("%06d.hint", fileId))
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &HintFile{
		fileId: fileId,
		file:   f,
		offset: stat.Size(),
	}, nil
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
