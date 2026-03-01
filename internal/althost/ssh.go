package althost

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type SSHTunnel struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	ForwardsTo string `yaml:"forwards_to"`
	Reconnect  *bool  `yaml:"reconnect"` // defaults to true if nil

	// Runtime fields (not exported)
	mu         sync.Mutex
	socketPath string
	cmd        *exec.Cmd
	running    bool
	proxy      *httputil.ReverseProxy
	workDir    string
}

func (t *SSHTunnel) SetWorkDir(dir string) {
	t.workDir = dir
}

func (t *SSHTunnel) Open() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return nil
	}

	if t.Port == 0 {
		t.Port = 22
	}

	// Create temp socket file in workdir
	socketFile := fmt.Sprintf("tunnel_%s_%d.sock", sanitizeHost(t.Host), time.Now().UnixNano())
	t.socketPath = filepath.Join(t.workDir, socketFile)

	// Parse forwards_to (format: host:port)
	remoteParts := strings.Split(t.ForwardsTo, ":")
	if len(remoteParts) != 2 {
		return fmt.Errorf("invalid forwards_to format: %s, expected host:port", t.ForwardsTo)
	}
	remoteHost := remoteParts[0]
	remotePort := remoteParts[1]

	// Build SSH command: ssh -N -L /path/to/socket:remote_host:remote_port ssh_host -p ssh_port
	t.cmd = exec.Command("ssh",
		"-N",
		"-o", "ServerAliveInterval=60",
		"-o", "ServerAliveCountMax=3",
		"-L", fmt.Sprintf("%s:%s:%s", t.socketPath, remoteHost, remotePort),
		t.Host,
		"-p", fmt.Sprintf("%d", t.Port),
	)

	// Start SSH process
	if err := t.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start SSH tunnel: %w", err)
	}

	t.running = true

	// Wait for socket to be created (with timeout)
	if err := t.waitForSocket(10 * time.Second); err != nil {
		t.Close()
		return fmt.Errorf("SSH tunnel failed to create socket: %w", err)
	}

	// Setup reverse proxy transport to use UNIX socket
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", t.socketPath)
		},
	}

	// Create reverse proxy
	director := func(req *http.Request) {
		req.URL.Scheme = "http"
		req.URL.Host = t.ForwardsTo
		req.Host = t.ForwardsTo
	}
	t.proxy = &httputil.ReverseProxy{
		Director:  director,
		Transport: transport,
	}

	// Handle reconnection in background if enabled (default true)
	shouldReconnect := true
	if t.Reconnect != nil {
		shouldReconnect = *t.Reconnect
	}
	if shouldReconnect {
		go t.monitorAndReconnect()
	}

	return nil
}

func (t *SSHTunnel) waitForSocket(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(t.socketPath); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for socket %s", t.socketPath)
}

func (t *SSHTunnel) monitorAndReconnect() {
	err := t.cmd.Wait()

	t.mu.Lock()
	wasRunning := t.running
	t.running = false
	t.mu.Unlock()

	if wasRunning {
		if err != nil {
			log.Printf("SSH tunnel to %s exited: %v", t.Host, err)
		}
		
		// Check if we should reconnect
		shouldReconnect := true
		if t.Reconnect != nil {
			shouldReconnect = *t.Reconnect
		}
		
		if shouldReconnect {
			log.Printf("Reconnecting SSH tunnel to %s...", t.Host)
			time.Sleep(1 * time.Second) // Brief pause before reconnect
			if err := t.Open(); err != nil {
				log.Printf("Failed to reconnect SSH tunnel to %s: %v", t.Host, err)
			}
		}
	}
}

func (t *SSHTunnel) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return
	}

	t.running = false

	if t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Kill()
		t.cmd.Wait()
	}

	if t.socketPath != "" {
		os.Remove(t.socketPath)
	}
}

func (t *SSHTunnel) Forward(w http.ResponseWriter, r *http.Request) {
	t.mu.Lock()
	proxy := t.proxy
	running := t.running
	t.mu.Unlock()

	if !running {
		http.Error(w, "SSH tunnel not running", http.StatusServiceUnavailable)
		return
	}

	proxy.ServeHTTP(w, r)
}

func sanitizeHost(host string) string {
	// Replace characters that might be problematic in filenames
	return strings.ReplaceAll(strings.ReplaceAll(host, ".", "_"), ":", "_")
}
