package main

import (
	"encoding/binary"
	"hash/crc32"
	"io"
	"math"
	"time"
)

const (
	// HeaderSize is the fixed size of a record header on disk:
	// 4 bytes CRC32 + 8 bytes timestamp + 4 bytes keySize + 4 bytes valueSize
	HeaderSize = 20

	// TombstoneValue is the sentinel value stored in valueSize to indicate a deleted key.
	TombstoneValue = math.MaxUint32
)

type Record struct {
	crc       uint32
	timestamp uint64
	keySize   uint32
	valueSize uint32
	key       []byte
	value     []byte
}

func NewRecord(key, value []byte) *Record {
	r := &Record{
		timestamp: uint64(time.Now().UnixNano()),
		keySize:   uint32(len(key)),
		valueSize: uint32(len(value)),
		key:       key,
		value:     value,
	}

	buf := r.Encode()
	r.crc = crc32.ChecksumIEEE(buf[4:])
	return r
}

// NewTombstoneRecord creates a tombstone record for deletion.
// The valueSize is set to TombstoneValue and no value bytes are stored on disk.
func NewTombstoneRecord(key []byte) *Record {
	r := &Record{
		timestamp: uint64(time.Now().UnixNano()),
		keySize:   uint32(len(key)),
		valueSize: TombstoneValue,
		key:       key,
		value:     nil,
	}

	buf := r.Encode()
	r.crc = crc32.ChecksumIEEE(buf[4:])
	return r
}

func (r *Record) Encode() []byte {
	valueLen := len(r.value)
	buf := make([]byte, HeaderSize+len(r.key)+valueLen)
	binary.BigEndian.PutUint32(buf[0:4], r.crc)
	binary.BigEndian.PutUint64(buf[4:12], r.timestamp)
	binary.BigEndian.PutUint32(buf[12:16], r.keySize)
	binary.BigEndian.PutUint32(buf[16:20], r.valueSize)
	copy(buf[HeaderSize:], r.key)
	if valueLen > 0 {
		copy(buf[HeaderSize+len(r.key):], r.value)
	}
	return buf
}

// RecordSize returns the total on-disk size of this record.
func (r *Record) RecordSize() int64 {
	if r.valueSize == TombstoneValue {
		return int64(HeaderSize) + int64(r.keySize)
	}
	return int64(HeaderSize) + int64(r.keySize) + int64(r.valueSize)
}

func DecodeRecord(r io.Reader) (*Record, error) {
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	crc := binary.BigEndian.Uint32(header[0:4])
	timestamp := binary.BigEndian.Uint64(header[4:12])
	keySize := binary.BigEndian.Uint32(header[12:16])
	valueSize := binary.BigEndian.Uint32(header[16:20])

	key := make([]byte, keySize)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}

	var value []byte
	if valueSize == TombstoneValue {
		value = nil
	} else {
		value = make([]byte, valueSize)
		if _, err := io.ReadFull(r, value); err != nil {
			return nil, err
		}
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

func ValidateCrc(r *Record) bool {
	buf := r.Encode()
	computed := crc32.ChecksumIEEE(buf[4:])
	return computed == r.crc
}
