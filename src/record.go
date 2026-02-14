package main

import (
	"encoding/binary"
	"hash/crc64"
	"io"
)
type Record struct {
	crc       uint64
	timestamp uint64
	keySize   uint32
	valueSize uint32
	key       []byte
	value     []byte
}

func (r *Record) Encode() []byte {
	headerSize := 24
    buf := make([]byte, headerSize + len(r.key) + len(r.value))
    binary.BigEndian.PutUint64(buf[0:8], r.crc)
    binary.BigEndian.PutUint64(buf[8:16], r.timestamp)
    binary.BigEndian.PutUint32(buf[16:20], r.keySize)
    binary.BigEndian.PutUint32(buf[20:24], r.valueSize)
    copy(buf[24:], r.key)
    copy(buf[24+len(r.key):], r.value)
    return buf
  }

func DecodeRecord(r io.Reader) (*Record, error) {
	header := make([]byte, 24)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	crc := binary.BigEndian.Uint64(header[0:8])
	timestamp := binary.BigEndian.Uint64(header[8:16])
	keySize := binary.BigEndian.Uint32(header[16:20])
	valueSize := binary.BigEndian.Uint32(header[20:24])

	key := make([]byte, keySize)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}

	value := make([]byte, valueSize)
	if _, err := io.ReadFull(r, value); err != nil {
		return nil, err
	}

	return &Record{
		crc:       crc,
		timestamp: timestamp,
		keySize:   keySize,
		valueSize: valueSize,
		key:       key,
		value:     value,
	}, nil
}

func ValidateCrc(r *Record) (bool, error) {
	buf := r.Encode()
	table := crc64.MakeTable(crc64.ECMA)
	computed := crc64.Checksum(buf[8:], table)
	return computed == r.crc, nil
}
