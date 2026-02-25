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

type ServiceType int

const (
	ServiceTypeUnknown ServiceType = iota
	ServiceTypeFiles
	ServiceTypeProxy
	ServiceTypeAPI
)

type Service struct {
	Subdomain  string   `yaml:"subdomain"`
	ServeFiles string   `yaml:"serve_files"`
	ForwardsTo string   `yaml:"forwards_to"`
	API        bool     `yaml:"api"`
	Start      []string `yaml:"start"`
	Stop       []string `yaml:"stop"`
	Timeout    int      `yaml:"timeout"`
}

func (s *Service) Type() ServiceType {
	if s.ServeFiles != "" {
		return ServiceTypeFiles
	}
	if s.ForwardsTo != "" {
		return ServiceTypeProxy
	}
	if s.API {
		return ServiceTypeAPI
	}

	return ServiceTypeUnknown
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

	for name, svc := range cfg.Services {
		if svc.Type() == ServiceTypeUnknown {
			return nil, fmt.Errorf("service %s: one of serve_files, forwards_to, or api must be set", name)
		}
	}

	return &cfg, nil
}
