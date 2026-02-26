package config

import (
	"fmt"
	"os"
	"path/filepath"
	"serveroute/internal/service"

	"github.com/goccy/go-yaml"
)

type Config struct {
	Listen struct {
		HTTP  string `yaml:"http"`
		HTTPS string `yaml:"https"`
	} `yaml:"listen"`
	SSLCertificate      string                      `yaml:"ssl_certificate"`
	SSLCertificateKey   string                      `yaml:"ssl_certificate_key"`
	Domain              string                      `yaml:"domain"`
	WorkDir             string                      `yaml:"workdir"`
	Services            map[string]*service.Service `yaml:"services"`
	ServicesBySubdomain map[string]service.NamedService
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

	// Set default workdir to config file directory if not specified
	configFileDir := filepath.Dir(path)
	configFileDir, err = filepath.Abs(configFileDir)
	if err != nil {
		return nil, fmt.Errorf("cannot find abs path for config dir")
	}

	if cfg.WorkDir == "" {
		cfg.WorkDir = configFileDir
	} else {
		// If workdir is relative, make it relative to config file path
		if !filepath.IsAbs(cfg.WorkDir) {
			cfg.WorkDir = filepath.Join(configFileDir, cfg.WorkDir)
		}
	}

	for name, svc := range cfg.Services {
		if svc.Type() == service.ServiceTypeUnknown {
			return nil, fmt.Errorf("service %s: one of serve_files, forwards_to, or api must be set", name)
		}
	}

	cfg.ServicesBySubdomain = service.MakeServicesBySubdomain(cfg.Services)

	return &cfg, nil
}
