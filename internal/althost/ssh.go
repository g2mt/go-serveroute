package althost

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"sync"
	"time"
)

type SSHTunnel struct {
	Host                  string `yaml:"host"`
	ForwardsTo            string `yaml:"forwards_to"`
	Reconnect             *bool  `yaml:"reconnect"` // defaults to true if nil
	InsecureSkipVerifyTLS bool   `yaml:"insecure_skip_verify_tls"`

	mu         sync.Mutex
	stopped    bool
	socketPath string
	proxy      *httputil.ReverseProxy
	cmd        *exec.Cmd
	ctx        context.Context
	cancel     context.CancelFunc
}

func (t *SSHTunnel) unlockedShouldReconnect() bool {
	if t.stopped {
		return false
	}
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

	// Create temp socket file
	socketDir, err := os.MkdirTemp("", "serveroute_tun.*")
	if err != nil {
		return err
	}
	t.socketPath = path.Join(socketDir, "socket")

	// Build SSH command
	remoteUrl, err := url.Parse(t.ForwardsTo)
	if err != nil {
		return fmt.Errorf("parsing target URL: %w", err)
	}

	// Create context for command
	t.ctx, t.cancel = context.WithCancel(context.Background())
	t.cmd = exec.CommandContext(t.ctx, "ssh",
		"-N",
		"-o", "ServerAliveInterval=60",
		"-o", "ServerAliveCountMax=3",
		"-L", fmt.Sprintf("%s:%s", t.socketPath, remoteUrl.Host),
		t.Host,
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
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: t.InsecureSkipVerifyTLS,
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
	if t.unlockedShouldReconnect() {
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
	t.mu.Lock()
	ctx := t.ctx
	t.mu.Unlock()

	select {
	case _ = <-ctx.Done():
		t.mu.Lock()
		defer t.mu.Unlock()

		log.Printf("SSH tunnel to %s exited", t.Host)

		if t.unlockedShouldReconnect() {
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

	t.stopped = true
	if t.cmd != nil {
		t.cmd.Process.Kill()
		t.cancel()
	}
	if t.socketPath != "" {
		os.RemoveAll(path.Dir(t.socketPath))
	}
}

func (t *SSHTunnel) Forward(w http.ResponseWriter, r *http.Request) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.proxy.ServeHTTP(w, r)
}
