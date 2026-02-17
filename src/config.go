package main

import (
	"encoding/json"
	"os"
)

type SyncStrategy string

const (
	SyncAlways   SyncStrategy = "always"
	SyncNone     SyncStrategy = "none"
	SyncInterval SyncStrategy = "interval"
)

type Config struct {
	Directory    string       `json:"directory"`
	MaxFileSize  int64        `json:"max_file_size"`
	SyncStrategy SyncStrategy `json:"sync_strategy"`
	SyncInterval int          `json:"sync_interval"` // milliseconds, for "interval" mode
}

func DefaultConfig(dir string) Config {
	return Config{
		Directory:    dir,
		MaxFileSize:  256 * 1024 * 1024, // 256 MB
		SyncStrategy: SyncNone,
		SyncInterval: 0,
	}
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	cfg := DefaultConfig("")
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
