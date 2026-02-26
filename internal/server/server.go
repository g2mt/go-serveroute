package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"serveroute/internal/config"
	"serveroute/internal/event"
	"serveroute/internal/service"
)

type Server struct {
	Config   *config.Config // readonly
	Services map[string]*service.ServiceState
	Mu       sync.RWMutex
	EventBus *event.EventBus
}

func NewServer(cfg *config.Config) *Server {
	return &Server{
		Config:   cfg,
		Services: make(map[string]*service.ServiceState),
		EventBus: event.NewEventBus(),
	}
}

func (s *Server) cleanup() {
	for _, state := range s.Services {
		state.Mu.Lock()
		defer state.Mu.Unlock()
		state.EventBus = nil
		state.Stop()
	}
}

func (s *Server) ServeForever() {
	defer s.cleanup()

	go func() {
		http.HandleFunc("/", s.handleRequest)
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
	if svc.Type() == service.ServiceTypeProxy {
		if state.Timer != nil {
			state.Timer.Stop()
		}
		if svc.GetTimeout() > 0 {
			state.Timer = time.AfterFunc(time.Duration(svc.GetTimeout())*time.Second, func() {
				state.Stop()
			})
		}
	}
	state.Mu.Unlock()

	switch svc.Type() {
	case service.ServiceTypeAPI:
		s.handleAPI(w, r, state)
	case service.ServiceTypeFiles:
		s.serveFiles(w, r, svc.GetServeFiles())
	case service.ServiceTypeProxy:
		if err := state.Start(); err != nil {
			http.Error(w, fmt.Sprintf("Failed to start service: %v", err), http.StatusInternalServerError)
			return
		}
		s.proxyRequest(w, r, svc.GetForwardsTo())
	default:
		panic("Service not configured") // configure happens on load
	}
}

func (s *Server) findService(host string) (string, *service.Service) {
	host = strings.Split(host, ":")[0]

	var subdomain string
	domain := s.Config.Domain

	if domain != "" &&
		len(host) >= (len(domain)+1) &&
		strings.HasSuffix(host, domain) &&
		host[len(host)-len(domain)-1] == '.' {
		subdomain = host[:len(host)-len(domain)-1]
	} else if host == domain {
		subdomain = ""
	} else {
		parts := strings.Split(host, ".")
		if len(parts) >= 1 {
			subdomain = parts[0]
		} else {
			subdomain = ""
		}
	}

	if namedSvc, ok := s.Config.ServicesBySubdomain[subdomain]; ok {
		return namedSvc.Name, namedSvc.Svc
	}

	return "", nil
}

func (s *Server) getOrCreateState(name string, svc *service.Service) *service.ServiceState {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if state, ok := s.Services[name]; ok {
		return state
	}

	state := &service.ServiceState{
		Name:     name,
		Service:  svc,
		EventBus: s.EventBus,
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

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request, state *service.ServiceState) {
	w.Header().Set("Content-Type", "application/json")

	path := strings.TrimPrefix(r.URL.Path, "/")
	switch path {
	case "start":
		if err := state.Start(); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "error",
				"error":  err.Error(),
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
		})
	case "stop":
		state.Stop()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
		})
	case "status":
		running := state.IsRunning()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"running": running,
		})
	case "list":
		s.apiListServices(w)
	case "events":
		s.apiEvents(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "error",
			"error":  "Unknown API endpoint",
		})
	}
}

func (s *Server) apiListServices(w http.ResponseWriter) {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	result := make(map[string]interface{})

	for name, service := range s.Config.Services {
		if service.GetHidden() {
			continue
		}

		status := "stopped"
		if state, ok := s.Services[name]; ok {
			state.Mu.Lock()
			defer state.Mu.Unlock()

			if state.IsRunning() {
				status = "started"
			}
		}

		result[name] = map[string]interface{}{
			"status":    status,
			"subdomain": service.GetSubdomain(),
		}
	}

	json.NewEncoder(w).Encode(result)
}
