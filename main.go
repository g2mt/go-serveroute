package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"serveroute/internal/config"
	"serveroute/internal/server"
)

func main() {
	configPath := flag.String("config", "", "Path to config file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Changing directory to %s", cfg.WorkDir)
	if err := os.Chdir(cfg.WorkDir); err != nil {
		log.Fatalf("Failed to change to workdir %s: %v", cfg.WorkDir, err)
	}

	server := server.NewServer(cfg)

	if err := server.StartAuto(); err != nil {
		log.Fatalf("Failed to autostart services: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go server.ServeForever()

	<-ctx.Done()
	server.Close()
}
