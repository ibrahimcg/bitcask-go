package main

import (
	"encoding/binary"
	"io"
)

// HintEntryHeaderSize is the fixed header for a hint entry:
// 8 bytes timestamp + 4 bytes keySize + 4 bytes recordSize + 8 bytes offset = 24 bytes
const HintEntryHeaderSize = 24

type HintEntry struct {
	timestamp  uint64
	keySize    uint32
	recordSize uint32 // total record size on disk
	offset     int64  // byte offset of record start in data file
	key        []byte
}

func (h *HintEntry) Encode() []byte {
	buf := make([]byte, HintEntryHeaderSize+len(h.key))
	binary.BigEndian.PutUint64(buf[0:8], h.timestamp)
	binary.BigEndian.PutUint32(buf[8:12], h.keySize)
	binary.BigEndian.PutUint32(buf[12:16], h.recordSize)
	binary.BigEndian.PutUint64(buf[16:24], uint64(h.offset))
	copy(buf[HintEntryHeaderSize:], h.key)
	return buf
}

func DecodeHintEntry(r io.Reader) (*HintEntry, error) {
	header := make([]byte, HintEntryHeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	timestamp := binary.BigEndian.Uint64(header[0:8])
	keySize := binary.BigEndian.Uint32(header[8:12])
	recordSize := binary.BigEndian.Uint32(header[12:16])
	offset := int64(binary.BigEndian.Uint64(header[16:24]))

	key := make([]byte, keySize)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}

	return &HintEntry{
		timestamp:  timestamp,
		keySize:    keySize,
		recordSize: recordSize,
		offset:     offset,
		key:        key,
	}, nil
}
