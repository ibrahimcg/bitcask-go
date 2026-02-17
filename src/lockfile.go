package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const lockFileName = "db.lock"

type LockFile struct {
	file *os.File
}

// AcquireLock acquires an advisory lock on the data directory.
// Returns an error if another process already holds the lock.
func AcquireLock(dir string) (*LockFile, error) {
	path := filepath.Join(dir, lockFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("database is locked by another process: %w", err)
	}

	return &LockFile{file: f}, nil
}

// Release releases the advisory lock and removes the lock file.
func (lf *LockFile) Release() error {
	if lf.file == nil {
		return nil
	}

	path := lf.file.Name()
	syscall.Flock(int(lf.file.Fd()), syscall.LOCK_UN)
	lf.file.Close()
	os.Remove(path)
	lf.file = nil
	return nil
}
