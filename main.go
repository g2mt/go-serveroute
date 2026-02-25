package main

import (
	"flag"
	"log"
)

func main() {
	configPath := flag.String("config", "example.yaml", "Path to config file")
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	server := NewServer(cfg)
	server.Start()
}
