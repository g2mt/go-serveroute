package server

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"serveroute/internal/config"
	"serveroute/internal/event"
	"serveroute/internal/service"
)

type Server struct {
	Mu       sync.Mutex
	Config   *config.Config // readonly
	Services map[string]*service.ServiceState
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

	http.HandleFunc("/", s.handleRequest)

	if s.Config.Listen.HTTP != "" {
		go func() {
			log.Printf("Starting HTTP server on %s", s.Config.Listen.HTTP)
			if err := http.ListenAndServe(s.Config.Listen.HTTP, nil); err != nil {
				log.Fatalf("HTTP server error: %v", err)
			}
		}()
	}

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
		if svc.Timeout > 0 {
			state.Timer = time.AfterFunc(time.Duration(svc.Timeout)*time.Second, func() {
				state.Stop()
			})
		}
	}
	state.Mu.Unlock()

	switch svc.Type() {
	case service.ServiceTypeAPI:
		s.handleAPI(w, r, state)
	case service.ServiceTypeFiles:
		s.serveFiles(w, r, svc.ServeFiles)
	case service.ServiceTypeProxy:
		if err := state.Start(); err != nil {
			http.Error(w, fmt.Sprintf("Failed to start service: %v", err), http.StatusInternalServerError)
			return
		}
		s.proxyRequest(w, r, svc.ForwardsTo)
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
