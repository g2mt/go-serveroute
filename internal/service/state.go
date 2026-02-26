package service

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

	"serveroute/internal/event"
)

type ServiceState struct {
	Name     string
	Service  *Service
	Cmd      *exec.Cmd
	LastUsed time.Time
	Timer    *time.Timer
	Mu       sync.Mutex
	EventBus *event.EventBus
}

func (state *ServiceState) Start() error {
	if state.IsRunning() {
		return nil
	}

	state.Mu.Lock()
	defer state.Mu.Unlock()

	if len(state.Service.start) == 0 {
		return nil
	}

	log.Printf("Starting service %s: %v", state.Name, state.Service.start)

	cmd := exec.Command(state.Service.start[0], state.Service.start[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting service: %w", err)
	}

	state.Cmd = cmd

	if err := state.waitForService(); err != nil {
		return err
	}

	state.EventBus.Publish(event.Event{
		Type:    "start",
		Service: state.Name,
	})
	return nil
}

func (state *ServiceState) waitForService() error {
	target := state.Service.forwardsTo
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

func (state *ServiceState) Stop() {
	state.Mu.Lock()
	defer state.Mu.Unlock()

	if state.Cmd == nil || state.Cmd.Process == nil {
		return
	}

	log.Printf("Stopping service %s", state.Name)

	if len(state.Service.stop) > 0 {
		cmd := exec.Command(state.Service.stop[0], state.Service.stop[1:]...)
		cmd.Run()
	} else if state.Service.killTimeout > 0 {
		// Try graceful shutdown first
		if err := state.Cmd.Process.Signal(os.Interrupt); err != nil {
			log.Printf("Failed to send SIGINT to service %s: %v", state.Name, err)
			state.Cmd.Process.Kill()
		} else {
			// Wait for process to exit or timeout
			done := make(chan error, 1)
			go func() {
				done <- state.Cmd.Wait()
			}()

			select {
			case <-time.After(time.Duration(state.Service.killTimeout) * time.Second):
				log.Printf("Service %s shutdown timeout, killing process", state.Name)
				state.Cmd.Process.Kill()
				<-done // Wait for process to be killed
			case <-done:
				// Process exited normally
			}
		}
	} else {
		state.Cmd.Process.Kill()
	}

	if state.Cmd != nil && state.Cmd.ProcessState == nil {
		state.Cmd.Wait()
	}
	state.Cmd = nil

	state.EventBus.Publish(event.Event{
		Type:    "stop",
		Service: state.Name,
	})
}

func (state *ServiceState) IsRunning() bool {
	state.Mu.Lock()
	defer state.Mu.Unlock()

	switch state.Service.Type() {
	case ServiceTypeProxy:
		return state.Cmd != nil && state.Cmd.Process != nil && state.Cmd.ProcessState == nil
	default:
		return true
	}
}
