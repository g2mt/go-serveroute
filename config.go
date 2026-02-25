package main

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

type Config struct {
	Listen struct {
		HTTP  string `yaml:"http"`
		HTTPS string `yaml:"https"`
	} `yaml:"listen"`
	SSLCertificate    string             `yaml:"ssl_certificate"`
	SSLCertificateKey string             `yaml:"ssl_certificate_key"`
	Services          map[string]Service `yaml:"services"`
}

type Service struct {
	Subdomain  string   `yaml:"subdomain"`
	ServeFiles string   `yaml:"serve_files"`
	ForwardsTo string   `yaml:"forwards_to"`
	Start      []string `yaml:"start"`
	Stop       []string `yaml:"stop"`
	Timeout    int      `yaml:"timeout"`
	API        bool     `yaml:"api"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.Listen.HTTP == "" {
		return nil, fmt.Errorf("http listen address is required")
	}

	return &cfg, nil
}
