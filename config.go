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
	SSLCertificate      string              `yaml:"ssl_certificate"`
	SSLCertificateKey   string              `yaml:"ssl_certificate_key"`
	Domain              string              `yaml:"domain"`
	Services            map[string]*Service `yaml:"services"`
	servicesBySubdomain map[string]NamedService
}

func LoadConfig(path string) (*Config, error) {
	if path == "" {
		return nil, fmt.Errorf("config file is required")
	}

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

	// Initialize servicesBySubdomain for faster lookup
	cfg.servicesBySubdomain = make(map[string]NamedService)
	for name, svc := range cfg.Services {
		cfg.servicesBySubdomain[svc.Subdomain] = NamedService{name: name, svc: svc}
	}

	return &cfg, nil
}
