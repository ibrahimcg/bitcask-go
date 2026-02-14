package main

import (
	"encoding/binary"
	"io"
)

type HintEntry struct {
	timestamp uint64
	keySize   uint32
	valueSize uint32
	offset    int64
	key       []byte
}

func (h *HintEntry) Encode() []byte {
	headerSize := 24
	buf := make([]byte, headerSize+len(h.key))
	binary.BigEndian.PutUint64(buf[0:8], h.timestamp)
	binary.BigEndian.PutUint32(buf[8:12], h.keySize)
	binary.BigEndian.PutUint32(buf[12:16], h.valueSize)
	binary.BigEndian.PutUint64(buf[16:24], uint64(h.offset))
	copy(buf[24:], h.key)
	return buf
}

func DecodeHintEntry(r io.Reader) (*HintEntry, error) {
	header := make([]byte, 24)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	timestamp := binary.BigEndian.Uint64(header[0:8])
	keySize := binary.BigEndian.Uint32(header[8:12])
	valueSize := binary.BigEndian.Uint32(header[12:16])
	offset := int64(binary.BigEndian.Uint64(header[16:24]))

	key := make([]byte, keySize)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}

	return &HintEntry{
		timestamp: timestamp,
		keySize:   keySize,
		valueSize: valueSize,
		offset:    offset,
		key:       key,
	}, nil
}
