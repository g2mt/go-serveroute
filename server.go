package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Server struct {
	Config   *Config
	Services map[string]*ServiceState
	Mu       sync.RWMutex
}

func NewServer(cfg *Config) *Server {
	return &Server{
		Config:   cfg,
		Services: make(map[string]*ServiceState),
	}
}

func (s *Server) Start() error {
	http.HandleFunc("/", s.handleRequest)

	go func() {
		log.Printf("Starting HTTP server on %s", s.Config.Listen.HTTP)
		if err := http.ListenAndServe(s.Config.Listen.HTTP, nil); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	if s.Config.Listen.HTTPS != "" && s.Config.SSLCertificate != "" && s.Config.SSLCertificateKey != "" {
		go func() {
			log.Printf("Starting HTTPS server on %s", s.Config.Listen.HTTPS)
			if err := http.ListenAndServeTLS(s.Config.Listen.HTTPS, s.Config.SSLCertificate, s.Config.SSLCertificateKey, nil); err != nil {
				log.Fatalf("HTTPS server error: %v", err)
			}
		}()
	}

	select {}
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	svcName, svc := s.findService(host)
	if svc == nil {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	state := s.getOrCreateState(svcName, svc)

	state.Mu.Lock()
	state.LastUsed = time.Now()
	if svc.Type() == ServiceTypeProxy {
		if state.Timer != nil {
			state.Timer.Stop()
		}
		if svc.Timeout > 0 {
			state.Timer = time.AfterFunc(time.Duration(svc.Timeout)*time.Second, func() {
				state.stop()
			})
		}
	}
	state.Mu.Unlock()

	switch svc.Type() {
	case ServiceTypeAPI:
		s.handleAPI(w, r, state)
	case ServiceTypeFiles:
		s.serveFiles(w, r, svc.ServeFiles)
	case ServiceTypeProxy:
		if err := state.ensureRunningProcess(); err != nil {
			http.Error(w, fmt.Sprintf("Failed to start service: %v", err), http.StatusInternalServerError)
			return
		}
		s.proxyRequest(w, r, svc.ForwardsTo)
	default:
		panic("Service not configured") // configure happens on load
	}
}

func (s *Server) findService(host string) (string, *Service) {
	host = strings.Split(host, ":")[0]
	parts := strings.Split(host, ".")

	var subdomain string
	if len(parts) >= 2 {
		subdomain = parts[len(parts)-2]
	} else {
		subdomain = ""
	}

	for name, svc := range s.Config.Services {
		if svc.Subdomain == subdomain {
			return name, &svc
		}
	}

	for name, svc := range s.Config.Services {
		if svc.Subdomain == "" {
			return name, &svc
		}
	}

	return "", nil
}

func (s *Server) getOrCreateState(name string, svc *Service) *ServiceState {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if state, ok := s.Services[name]; ok {
		return state
	}

	state := &ServiceState{
		Name:    name,
		Service: svc,
	}
	s.Services[name] = state
	return state
}

func (s *Server) serveFiles(w http.ResponseWriter, r *http.Request, path string) {
	fs := http.FileServer(http.Dir(path))
	http.StripPrefix("/", fs).ServeHTTP(w, r)
}

func (s *Server) proxyRequest(w http.ResponseWriter, r *http.Request, target string) {
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}

	url, err := url.Parse(target)
	if err != nil {
		http.Error(w, "Invalid target URL", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(url)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
			clientIP = prior + ", " + clientIP
		}
		req.Header.Set("X-Forwarded-For", clientIP)
		req.Header.Set("X-Real-IP", r.RemoteAddr)

		if r.TLS != nil {
			req.Header.Set("X-Forwarded-Proto", "https")
		} else {
			req.Header.Set("X-Forwarded-Proto", "http")
		}
	}

	proxy.ServeHTTP(w, r)
}

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request, state *ServiceState) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	switch path {
	case "start":
		if err := state.ensureRunningProcess(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		io.WriteString(w, "started")
	case "stop":
		state.stop()
		io.WriteString(w, "stopped")
	case "status":
		state.Mu.Lock()
		running := state.Cmd != nil && state.Cmd.Process != nil && state.Cmd.ProcessState == nil
		state.Mu.Unlock()
		if running {
			io.WriteString(w, "running")
		} else {
			io.WriteString(w, "stopped")
		}
	default:
		http.Error(w, "Unknown API endpoint", http.StatusNotFound)
	}
}
