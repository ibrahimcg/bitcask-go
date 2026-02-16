package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type DataFile struct {
	fileId     uint32
	file       *os.File
	offset     int64
	isReadOnly bool
}

func NewDataFile(dir string, fileId uint32) (*DataFile, error) {
	path := filepath.Join(dir, fmt.Sprintf("%d.data", fileId))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	return &DataFile{
		fileId: fileId,
		file:   f,
		offset: 0,
	}, nil
}

func OpenDataFile(dir string, fileId uint32) (*DataFile, error) {
	path := filepath.Join(dir, fmt.Sprintf("%d.data", fileId))
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &DataFile{
		fileId:     fileId,
		file:       f,
		offset:     stat.Size(),
		isReadOnly: true,
	}, nil
}

func (df *DataFile) AppendRecord(r *Record) (int64, error) {
	if df.isReadOnly {
		return 0, fmt.Errorf("cannot append to read-only file %d", df.fileId)
	}

	data := r.Encode()
	pos := df.offset

	n, err := df.file.WriteAt(data, pos)
	if err != nil {
		return 0, err
	}
	df.offset += int64(n)

	return pos, nil
}

func (df *DataFile) ReadRecord(offset int64) (*Record, error) {
	reader := io.NewSectionReader(df.file, offset, df.offset-offset)
	return DecodeRecord(reader)
}

func (df *DataFile) Close() error {
	return df.file.Close()
}
