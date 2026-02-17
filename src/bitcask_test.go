package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "bitcask-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func openBitcask(t *testing.T, dir string) *Bitcask {
	t.Helper()
	bc, err := NewBitcask(DefaultConfig(dir))
	if err != nil {
		t.Fatalf("failed to open bitcask: %v", err)
	}
	t.Cleanup(func() { bc.Close() })
	return bc
}

// --- Basic Put/Get ---

func TestPutAndGet(t *testing.T) {
	dir := tempDir(t)
	bc := openBitcask(t, dir)

	ok, err := bc.Put("hello", "world")
	if err != nil || !ok {
		t.Fatalf("Put failed: ok=%v err=%v", ok, err)
	}

	val, err := bc.Get("hello")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "world" {
		t.Fatalf("expected 'world', got %q", val)
	}
}

func TestGetMissing(t *testing.T) {
	dir := tempDir(t)
	bc := openBitcask(t, dir)

	_, err := bc.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestPutOverwrite(t *testing.T) {
	dir := tempDir(t)
	bc := openBitcask(t, dir)

	bc.Put("key", "v1")
	bc.Put("key", "v2")

	val, err := bc.Get("key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "v2" {
		t.Fatalf("expected 'v2', got %q", val)
	}
}

func TestEmptyValue(t *testing.T) {
	dir := tempDir(t)
	bc := openBitcask(t, dir)

	bc.Put("empty", "")
	val, err := bc.Get("empty")
	if err != nil {
		t.Fatalf("Get empty value failed: %v", err)
	}
	if val != "" {
		t.Fatalf("expected empty string, got %q", val)
	}
}

// --- Delete ---

func TestDelete(t *testing.T) {
	dir := tempDir(t)
	bc := openBitcask(t, dir)

	bc.Put("key", "value")
	ok, err := bc.Delete("key")
	if err != nil || !ok {
		t.Fatalf("Delete failed: ok=%v err=%v", ok, err)
	}

	_, err = bc.Get("key")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeletePreservesOtherKeys(t *testing.T) {
	dir := tempDir(t)
	bc := openBitcask(t, dir)

	bc.Put("a", "1")
	bc.Put("b", "2")
	bc.Put("c", "3")

	bc.Delete("b")

	val, err := bc.Get("a")
	if err != nil || val != "1" {
		t.Fatalf("key 'a' should still exist: val=%q err=%v", val, err)
	}
	val, err = bc.Get("c")
	if err != nil || val != "3" {
		t.Fatalf("key 'c' should still exist: val=%q err=%v", val, err)
	}
}

func TestDeleteNonexistent(t *testing.T) {
	dir := tempDir(t)
	bc := openBitcask(t, dir)

	_, err := bc.Delete("nope")
	if err == nil {
		t.Fatal("expected error deleting nonexistent key")
	}
}

// --- Startup Recovery ---

func TestStartupRecovery(t *testing.T) {
	dir := tempDir(t)

	// Open, write data, close
	bc, err := NewBitcask(DefaultConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	bc.Put("k1", "v1")
	bc.Put("k2", "v2")
	bc.Put("k3", "v3")
	bc.Delete("k2")
	bc.Close()

	// Reopen — keydir must be rebuilt from data files
	bc2, err := NewBitcask(DefaultConfig(dir))
	if err != nil {
		t.Fatalf("failed to reopen: %v", err)
	}
	defer bc2.Close()

	val, err := bc2.Get("k1")
	if err != nil || val != "v1" {
		t.Fatalf("k1: expected 'v1', got %q err=%v", val, err)
	}
	val, err = bc2.Get("k3")
	if err != nil || val != "v3" {
		t.Fatalf("k3: expected 'v3', got %q err=%v", val, err)
	}
	_, err = bc2.Get("k2")
	if err == nil {
		t.Fatal("k2 should be deleted after recovery")
	}
}

func TestStartupRecoveryEmptyValue(t *testing.T) {
	dir := tempDir(t)

	bc, err := NewBitcask(DefaultConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	bc.Put("empty", "")
	bc.Close()

	bc2, err := NewBitcask(DefaultConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer bc2.Close()

	val, err := bc2.Get("empty")
	if err != nil {
		t.Fatalf("empty value key should survive recovery: %v", err)
	}
	if val != "" {
		t.Fatalf("expected empty string, got %q", val)
	}
}

// --- File Rotation ---

func TestFileRotation(t *testing.T) {
	dir := tempDir(t)
	cfg := DefaultConfig(dir)
	cfg.MaxFileSize = 100 // very small to trigger rotation quickly

	bc, err := NewBitcask(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer bc.Close()

	// Write enough data to trigger multiple rotations
	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("key-%03d", i)
		value := fmt.Sprintf("value-%03d", i)
		bc.Put(key, value)
	}

	// Should have multiple data files
	ids, _ := getDataFileIds(dir)
	if len(ids) < 2 {
		t.Fatalf("expected multiple data files, got %d", len(ids))
	}

	// All keys should still be readable
	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("key-%03d", i)
		expected := fmt.Sprintf("value-%03d", i)
		val, err := bc.Get(key)
		if err != nil {
			t.Fatalf("Get(%s) failed: %v", key, err)
		}
		if val != expected {
			t.Fatalf("Get(%s) = %q, want %q", key, val, expected)
		}
	}
}

func TestFileRotationWithRecovery(t *testing.T) {
	dir := tempDir(t)
	cfg := DefaultConfig(dir)
	cfg.MaxFileSize = 100

	bc, err := NewBitcask(cfg)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 20; i++ {
		bc.Put(fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i))
	}
	bc.Close()

	// Reopen and verify
	bc2, err := NewBitcask(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer bc2.Close()

	for i := 0; i < 20; i++ {
		val, err := bc2.Get(fmt.Sprintf("k%d", i))
		if err != nil {
			t.Fatalf("recovery failed for k%d: %v", i, err)
		}
		if val != fmt.Sprintf("v%d", i) {
			t.Fatalf("k%d: got %q, want %q", i, val, fmt.Sprintf("v%d", i))
		}
	}
}

// --- Merge/Compaction ---

func TestMerge(t *testing.T) {
	dir := tempDir(t)
	cfg := DefaultConfig(dir)
	cfg.MaxFileSize = 100

	bc, err := NewBitcask(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Write data to create multiple files
	for i := 0; i < 20; i++ {
		bc.Put(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i))
	}

	// Overwrite some keys (old values become stale)
	for i := 0; i < 10; i++ {
		bc.Put(fmt.Sprintf("key%d", i), fmt.Sprintf("updated%d", i))
	}

	// Delete some keys
	bc.Delete("key15")
	bc.Delete("key16")

	err = bc.Merge()
	if err != nil {
		t.Fatalf("Merge failed: %v", err)
	}

	// Verify all expected keys
	for i := 0; i < 10; i++ {
		val, err := bc.Get(fmt.Sprintf("key%d", i))
		if err != nil {
			t.Fatalf("Get key%d after merge: %v", i, err)
		}
		expected := fmt.Sprintf("updated%d", i)
		if val != expected {
			t.Fatalf("key%d: got %q, want %q", i, val, expected)
		}
	}
	for i := 10; i < 15; i++ {
		val, err := bc.Get(fmt.Sprintf("key%d", i))
		if err != nil {
			t.Fatalf("Get key%d after merge: %v", i, err)
		}
		expected := fmt.Sprintf("value%d", i)
		if val != expected {
			t.Fatalf("key%d: got %q, want %q", i, val, expected)
		}
	}

	// Deleted keys should remain gone
	_, err = bc.Get("key15")
	if err == nil {
		t.Fatal("key15 should be deleted after merge")
	}
	_, err = bc.Get("key16")
	if err == nil {
		t.Fatal("key16 should be deleted after merge")
	}

	bc.Close()
}

func TestMergeWithRecovery(t *testing.T) {
	dir := tempDir(t)
	cfg := DefaultConfig(dir)
	cfg.MaxFileSize = 100

	bc, err := NewBitcask(cfg)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 20; i++ {
		bc.Put(fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i))
	}

	bc.Merge()
	bc.Close()

	// Reopen and verify data survives merge + recovery
	bc2, err := NewBitcask(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer bc2.Close()

	for i := 0; i < 20; i++ {
		val, err := bc2.Get(fmt.Sprintf("k%d", i))
		if err != nil {
			t.Fatalf("k%d missing after merge+recovery: %v", i, err)
		}
		if val != fmt.Sprintf("v%d", i) {
			t.Fatalf("k%d: got %q, want %q", i, val, fmt.Sprintf("v%d", i))
		}
	}
}

// --- CRC Corruption Detection ---

func TestCRCCorruptionDetected(t *testing.T) {
	dir := tempDir(t)
	cfg := DefaultConfig(dir)
	cfg.MaxFileSize = 200

	bc, err := NewBitcask(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Write data and force it into a read-only file via rotation
	bc.Put("secret", "data")
	rmd := bc.keyDir.Get("secret")
	fileId := rmd.fileId
	offset := rmd.offset
	recordSize := rmd.size

	// Merge to create a hint file for this data
	// First we need a read-only file, so force rotation
	for i := 0; i < 20; i++ {
		bc.Put(fmt.Sprintf("pad%d", i), "padding-value")
	}
	bc.Merge()

	// After merge, secret should still be readable
	val, err := bc.Get("secret")
	if err != nil || val != "data" {
		t.Fatalf("pre-corruption check failed: val=%q err=%v", val, err)
	}

	// Now get the merged location
	rmd = bc.keyDir.Get("secret")
	fileId = rmd.fileId
	offset = rmd.offset
	recordSize = rmd.size
	_ = recordSize

	bc.Close()

	// Corrupt a byte in the value area of the merged data file
	path := filepath.Join(dir, fmt.Sprintf("%06d.data", fileId))
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		t.Fatal(err)
	}
	corruptOffset := offset + int64(HeaderSize) + int64(len("secret"))
	buf := make([]byte, 1)
	f.ReadAt(buf, corruptOffset)
	buf[0] ^= 0xFF
	f.WriteAt(buf, corruptOffset)
	f.Close()

	// Reopen — keydir rebuilds from hint file (which has no CRC),
	// so the key will be in the keydir but the data is corrupt
	bc2, err := NewBitcask(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer bc2.Close()

	_, err = bc2.Get("secret")
	if err == nil {
		t.Fatal("expected CRC error for corrupted record")
	}
	if !strings.Contains(err.Error(), "CRC") {
		t.Fatalf("expected CRC error, got: %v", err)
	}
}

// --- Lock File ---

func TestLockFilePreventsDoubleOpen(t *testing.T) {
	dir := tempDir(t)

	bc, err := NewBitcask(DefaultConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer bc.Close()

	// Try to open a second instance — should fail
	_, err = NewBitcask(DefaultConfig(dir))
	if err == nil {
		t.Fatal("expected lock error for double open")
	}
	if !strings.Contains(err.Error(), "locked") {
		t.Fatalf("expected lock error, got: %v", err)
	}
}

func TestLockReleasedOnClose(t *testing.T) {
	dir := tempDir(t)

	bc, err := NewBitcask(DefaultConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	bc.Put("x", "y")
	bc.Close()

	// Should be able to reopen after close
	bc2, err := NewBitcask(DefaultConfig(dir))
	if err != nil {
		t.Fatalf("failed to reopen after close: %v", err)
	}
	defer bc2.Close()

	val, _ := bc2.Get("x")
	if val != "y" {
		t.Fatalf("expected 'y', got %q", val)
	}
}

// --- ListKeys ---

func TestListKeys(t *testing.T) {
	dir := tempDir(t)
	bc := openBitcask(t, dir)

	bc.Put("c", "3")
	bc.Put("a", "1")
	bc.Put("b", "2")

	keys := bc.ListKeys()
	sort.Strings(keys)

	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Fatalf("unexpected keys: %v", keys)
	}
}

func TestListKeysAfterDelete(t *testing.T) {
	dir := tempDir(t)
	bc := openBitcask(t, dir)

	bc.Put("a", "1")
	bc.Put("b", "2")
	bc.Delete("a")

	keys := bc.ListKeys()
	if len(keys) != 1 || keys[0] != "b" {
		t.Fatalf("expected ['b'], got %v", keys)
	}
}

// --- Fold ---

func TestFold(t *testing.T) {
	dir := tempDir(t)
	bc := openBitcask(t, dir)

	bc.Put("a", "1")
	bc.Put("b", "2")
	bc.Put("c", "3")

	collected := make(map[string]string)
	err := bc.Fold(func(key, value string) error {
		collected[key] = value
		return nil
	})
	if err != nil {
		t.Fatalf("Fold error: %v", err)
	}

	if len(collected) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(collected))
	}
	if collected["a"] != "1" || collected["b"] != "2" || collected["c"] != "3" {
		t.Fatalf("unexpected fold result: %v", collected)
	}
}

func TestFoldStopsOnError(t *testing.T) {
	dir := tempDir(t)
	bc := openBitcask(t, dir)

	bc.Put("a", "1")
	bc.Put("b", "2")

	count := 0
	err := bc.Fold(func(key, value string) error {
		count++
		return fmt.Errorf("stop")
	})
	if err == nil {
		t.Fatal("expected error from Fold")
	}
	if count != 1 {
		t.Fatalf("expected fold to stop after 1 call, got %d", count)
	}
}

// --- Concurrent Access ---

func TestConcurrentReadsAndWrites(t *testing.T) {
	dir := tempDir(t)
	bc := openBitcask(t, dir)

	// Pre-populate
	for i := 0; i < 100; i++ {
		bc.Put(fmt.Sprintf("key%d", i), fmt.Sprintf("val%d", i))
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 200)

	// Concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key%d", j)
				_, err := bc.Get(key)
				if err != nil {
					errCh <- fmt.Errorf("Get(%s): %v", key, err)
					return
				}
			}
		}()
	}

	// Concurrent writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				key := fmt.Sprintf("writer%d-key%d", id, j)
				_, err := bc.Put(key, "value")
				if err != nil {
					errCh <- fmt.Errorf("Put(%s): %v", key, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
}

// --- Sync Strategy ---

func TestSyncAlways(t *testing.T) {
	dir := tempDir(t)
	cfg := DefaultConfig(dir)
	cfg.SyncStrategy = SyncAlways

	bc, err := NewBitcask(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer bc.Close()

	// Just verify it works without errors
	bc.Put("k", "v")
	val, _ := bc.Get("k")
	if val != "v" {
		t.Fatalf("expected 'v', got %q", val)
	}
}

func TestSyncInterval(t *testing.T) {
	dir := tempDir(t)
	cfg := DefaultConfig(dir)
	cfg.SyncStrategy = SyncInterval
	cfg.SyncInterval = 50

	bc, err := NewBitcask(cfg)
	if err != nil {
		t.Fatal(err)
	}

	bc.Put("k", "v")
	bc.Close() // should cleanly stop the sync worker
}

// --- Zero-Padded File Names ---

func TestZeroPaddedFileNames(t *testing.T) {
	dir := tempDir(t)
	bc := openBitcask(t, dir)

	bc.Put("test", "value")

	// Check that the data file has zero-padded name
	matches, _ := filepath.Glob(filepath.Join(dir, "*.data"))
	if len(matches) == 0 {
		t.Fatal("no data files found")
	}
	for _, m := range matches {
		name := filepath.Base(m)
		if !strings.HasPrefix(name, "0") {
			t.Fatalf("expected zero-padded filename, got %s", name)
		}
	}
}

// --- Record Format ---

func TestRecordEncodeDecode(t *testing.T) {
	r := NewRecord([]byte("hello"), []byte("world"))
	if r.keySize != 5 || r.valueSize != 5 {
		t.Fatalf("unexpected sizes: key=%d value=%d", r.keySize, r.valueSize)
	}
	if !ValidateCrc(r) {
		t.Fatal("CRC validation failed for new record")
	}
}

func TestTombstoneRecord(t *testing.T) {
	r := NewTombstoneRecord([]byte("deleted"))
	if r.valueSize != TombstoneValue {
		t.Fatalf("expected TombstoneValue, got %d", r.valueSize)
	}
	if r.value != nil {
		t.Fatal("tombstone value should be nil")
	}
	if !ValidateCrc(r) {
		t.Fatal("CRC validation failed for tombstone record")
	}

	// Verify on-disk size: HeaderSize + keySize, no value bytes
	if r.RecordSize() != int64(HeaderSize)+int64(len("deleted")) {
		t.Fatalf("unexpected record size: %d", r.RecordSize())
	}
}

func TestHeaderSizeConstant(t *testing.T) {
	if HeaderSize != 20 {
		t.Fatalf("HeaderSize should be 20, got %d", HeaderSize)
	}
}

// --- Multiple operations stress test ---

func TestManyOperations(t *testing.T) {
	dir := tempDir(t)
	cfg := DefaultConfig(dir)
	cfg.MaxFileSize = 512

	bc, err := NewBitcask(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Write
	for i := 0; i < 100; i++ {
		bc.Put(fmt.Sprintf("k%04d", i), fmt.Sprintf("v%04d", i))
	}

	// Delete half
	for i := 0; i < 50; i++ {
		bc.Delete(fmt.Sprintf("k%04d", i))
	}

	// Update remaining
	for i := 50; i < 100; i++ {
		bc.Put(fmt.Sprintf("k%04d", i), fmt.Sprintf("updated%04d", i))
	}

	// Merge
	if err := bc.Merge(); err != nil {
		t.Fatalf("merge: %v", err)
	}

	// Verify
	for i := 0; i < 50; i++ {
		_, err := bc.Get(fmt.Sprintf("k%04d", i))
		if err == nil {
			t.Fatalf("k%04d should be deleted", i)
		}
	}
	for i := 50; i < 100; i++ {
		val, err := bc.Get(fmt.Sprintf("k%04d", i))
		if err != nil {
			t.Fatalf("k%04d: %v", i, err)
		}
		expected := fmt.Sprintf("updated%04d", i)
		if val != expected {
			t.Fatalf("k%04d: got %q, want %q", i, val, expected)
		}
	}

	bc.Close()

	// Reopen and verify persistence
	bc2, err := NewBitcask(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer bc2.Close()

	keys := bc2.ListKeys()
	if len(keys) != 50 {
		t.Fatalf("expected 50 keys after recovery, got %d", len(keys))
	}
}
