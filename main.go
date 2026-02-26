package main

import (
	"flag"
	"log"
	"os"
)

func main() {
	configPath := flag.String("config", "", "Path to config file")
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := os.Chdir(cfg.WorkDir); err != nil {
		log.Fatalf("Failed to change to workdir %s: %v", cfg.WorkDir, err)
	}

	server := NewServer(cfg)
	server.Start()
}
