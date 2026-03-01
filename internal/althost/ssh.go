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
	"sync"
	"time"
)

type SSHTunnel struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	ForwardsTo string `yaml:"forwards_to"`
	Reconnect  *bool  `yaml:"reconnect"` // defaults to true if nil

	mu         sync.Mutex
	socketPath string
	cmd        *exec.Cmd
	done       chan struct{}
	proxy      *httputil.ReverseProxy
}

func (t *SSHTunnel) shouldReconnect() bool {
	shouldReconnect := true
	if t.Reconnect != nil {
		shouldReconnect = *t.Reconnect
	}
	return shouldReconnect
}

func (t *SSHTunnel) Open() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cmd != nil {
		return nil
	}

	if t.Port == 0 {
		t.Port = 22
	}

	// Create temp socket file
	socketFile, err := os.CreateTemp("", "serveroute_tun.*.socket")
	if err != nil {
		return err
	}
	t.socketPath = socketFile.Name()
	socketFile.Close()

	// Build SSH command: ssh -N -L /path/to/socket:remote_host:remote_port ssh_host -p ssh_port
	remoteUrl, err := url.Parse(t.ForwardsTo)
	if err != nil {
		return fmt.Errorf("parsing target URL: %w", err)
	}
	t.cmd = exec.Command("ssh",
		"-N",
		"-o", "ServerAliveInterval=60",
		"-o", "ServerAliveCountMax=3",
		"-L", fmt.Sprintf("%s:%s", t.socketPath, remoteUrl.Host),
		t.Host,
		"-p", fmt.Sprintf("%d", t.Port),
	)

	// Start SSH process
	if err := t.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start SSH tunnel: %w", err)
	}

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
		req.URL.Scheme = remoteUrl.Scheme
		req.URL.Host = t.ForwardsTo
		req.Host = t.ForwardsTo
	}
	t.proxy = &httputil.ReverseProxy{
		Director:  director,
		Transport: transport,
	}

	// Handle reconnection in background if enabled (default true)
	if t.shouldReconnect() {
		t.done = make(chan struct{})
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
	cmdFinished := make(chan struct{})

	go func() {
		t.mu.Lock()
		cmd := t.cmd
		t.mu.Unlock()

		cmd.Wait() // should be safe so long as cmd is not mutated
		cmdFinished <- struct{}{}
	}()

	go func() {
		t.mu.Lock()
		done := t.done
		defer t.mu.Unlock()

		select {
		case _ = <-cmdFinished:
			log.Printf("SSH tunnel to %s exited", t.Host)

			if t.shouldReconnect() {
				log.Printf("Reconnecting SSH tunnel to %s...", t.Host)
				time.Sleep(1 * time.Second) // Brief pause before reconnect
				if err := t.Open(); err != nil {
					log.Printf("Failed to reconnect SSH tunnel to %s: %v", t.Host, err)
				}
			}

		case _ = <-done:
		}
	}()
}

func (t *SSHTunnel) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.done != nil {
		t.done <- struct{}{} // stop the reconnection goroutine
	}

	if t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Kill()
	}

	if t.socketPath != "" {
		os.Remove(t.socketPath)
	}
}

func (t *SSHTunnel) Forward(w http.ResponseWriter, r *http.Request) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.proxy.ServeHTTP(w, r)
}
