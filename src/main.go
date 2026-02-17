package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	addr := flag.String("addr", ":6379", "listen address")
	dir := flag.String("dir", "/tmp/bitcask-data", "data directory")
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	var cfg Config
	if *configPath != "" {
		var err error
		cfg, err = LoadConfig(*configPath)
		if err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
		if *dir != "/tmp/bitcask-data" {
			cfg.Directory = *dir
		}
	} else {
		cfg = DefaultConfig(*dir)
	}

	bc, err := NewBitcask(cfg)
	if err != nil {
		log.Fatalf("failed to open bitcask: %v", err)
	}

	srv, err := NewServer(*addr, bc)
	if err != nil {
		bc.Close()
		log.Fatalf("failed to start server: %v", err)
	}

	log.Printf("bitcask server listening on %s", srv.Addr())

	go srv.Serve()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("shutting down...")
	srv.Close()
	if err := bc.Close(); err != nil {
		log.Printf("error closing bitcask: %v", err)
	}
	log.Println("goodbye")
}
