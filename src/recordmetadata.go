package main

type RecordMetadata struct {
	fileId    uint32
	offset    int64  // byte offset of record start in file
	size      uint32 // total record size on disk
	timestamp uint64
}
