package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type ServiceState struct {
	Name     string
	Service  *Service
	Cmd      *exec.Cmd
	LastUsed time.Time
	Timer    *time.Timer
	Mu       sync.Mutex
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

func (state *ServiceState) ensureRunningProcess() error {
	state.Mu.Lock()
	defer state.Mu.Unlock()

	if state.Cmd != nil && state.Cmd.Process != nil && state.Cmd.ProcessState == nil {
		return nil
	}

	if len(state.Service.Start) == 0 {
		return nil
	}

	log.Printf("Starting service %s: %v", state.Name, state.Service.Start)

	cmd := exec.Command(state.Service.Start[0], state.Service.Start[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting service: %w", err)
	}

	state.Cmd = cmd

	if err := state.waitForService(); err != nil {
		return err
	}

	return nil
}

func (state *ServiceState) waitForService() error {
	target := state.Service.ForwardsTo
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}

	url, err := url.Parse(target)
	if err != nil {
		return fmt.Errorf("parsing target URL: %w", err)
	}

	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		resp, err := http.Get(url.String() + "/")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("service did not start in time")
}

func (state *ServiceState) stop() {
	state.Mu.Lock()
	defer state.Mu.Unlock()

	if state.Cmd == nil || state.Cmd.Process == nil {
		return
	}

	log.Printf("Stopping service %s", state.Name)

	if len(state.Service.Stop) > 0 {
		cmd := exec.Command(state.Service.Stop[0], state.Service.Stop[1:]...)
		cmd.Run()
	} else {
		state.Cmd.Process.Kill()
	}

	state.Cmd.Wait()
	state.Cmd = nil
}
