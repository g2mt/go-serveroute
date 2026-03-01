package config

import (
	"fmt"
	"os"
	"path/filepath"
	"serveroute/internal/althost"
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
	Allowlist           []string                    `yaml:"allowlist"`
	Blocklist           []string                    `yaml:"blocklist"`
	Services            map[string]*service.Service `yaml:"services"`
	ServicesBySubdomain map[string]service.NamedService
	AltHosts            map[string]*althost.AltHost `yaml:"alt_hosts"`
}

func LoadConfig(path string) (*Config, error) {
	if path == "" {
		return nil, fmt.Errorf("config file is required")
	}

	configFileDir := filepath.Dir(path)
	configFileDir, err := filepath.Abs(configFileDir)
	if err != nil {
		return nil, fmt.Errorf("cannot find abs path for config dir")
	}

	// read config
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening config: %w", err)
	}

	var cfg Config
	dec := yaml.NewDecoder(file, yaml.ReferenceDirs(configFileDir))
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Set default workdir to config file directory if not specified
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
