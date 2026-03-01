package server

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"serveroute/internal/althost"
	"serveroute/internal/config"
	"serveroute/internal/event"
	"serveroute/internal/service"
)

func extractSubdomain(host, parentDomain string) (string, bool) {
	if parentDomain == "" {
		return "", false
	}
	if len(host) >= (len(parentDomain)+1) &&
		strings.HasSuffix(host, parentDomain) &&
		host[len(host)-len(parentDomain)-1] == '.' {
		return host[:len(host)-len(parentDomain)-1], true
	} else if host == parentDomain {
		return "", true
	} else {
		return "", false
	}
}

type Server struct {
	Mu       sync.Mutex     // global mutex, all methods should lock unless prefixed by "unlocked"
	Config   *config.Config // readonly
	Services map[string]*service.ServiceState
	AltHosts map[string]althost.Tunnel // host -> tunnel
	EventBus *event.EventBus
}

func NewServer(cfg *config.Config) *Server {
	return &Server{
		Config:   cfg,
		Services: make(map[string]*service.ServiceState),
		AltHosts: make(map[string]althost.Tunnel),
		EventBus: event.NewEventBus(),
	}
}

func (s *Server) StartAuto() error {
	for name, svc := range s.Config.Services {
		if svc.Autostart {
			state := s.getOrCreateState(service.NamedService{Name: name, Svc: svc})
			if err := state.Start(); err != nil {
				return fmt.Errorf("failed to start service %s: %w", name, err)
			}
		}
	}
	return nil
}

func (s *Server) cleanup() {
	for _, state := range s.Services {
		state.Mu.Lock()
		defer state.Mu.Unlock()
		state.EventBus = nil
		state.Stop()
	}

	// Close all SSH tunnels
	for host, tunnel := range s.AltHosts {
		log.Printf("Closing tunnel for %s", host)
		tunnel.Close()
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

func getClientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func matchesIPOrCIDR(ip net.IP, pattern string) bool {
	if strings.Contains(pattern, "/") {
		_, ipNet, err := net.ParseCIDR(pattern)
		if err != nil {
			return false
		}
		return ipNet.Contains(ip)
	}
	patternIP := net.ParseIP(pattern)
	if patternIP == nil {
		return false
	}
	return ip.Equal(patternIP)
}

func (s *Server) isIPAllowed(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, blocked := range s.Config.Blocklist {
		if matchesIPOrCIDR(ip, blocked) {
			return false
		}
	}
	if len(s.Config.Allowlist) > 0 {
		for _, allowed := range s.Config.Allowlist {
			if matchesIPOrCIDR(ip, allowed) {
				return true
			}
		}
		return false
	}
	return true
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	clientIP := getClientIP(r)
	if !s.isIPAllowed(clientIP) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	s.Mu.Lock()
	hostname := strings.Split(r.Host, ":")[0]
	if ah, ok := s.Config.AltHosts[hostname]; ok {
		s.Mu.Unlock()
		s.handleAltHost(w, r, hostname, ah)
		return
	}
	s.Mu.Unlock()

	namedSvc, ok := s.serviceByHostname(hostname)
	if !ok {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}
	state := s.getOrCreateState(namedSvc)
	svc := namedSvc.Svc

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
		s.handleAPI(w, r)
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

func (s *Server) serviceByName(name string) (service.NamedService, bool) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if svc, ok := s.Config.Services[name]; ok {
		return service.NamedService{Name: name, Svc: svc}, true
	} else {
		return service.NamedService{}, false
	}
}

func (s *Server) serviceByHostname(hostname string) (service.NamedService, bool) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	var subdomain string
	domain := s.Config.Domain

	if s, ok := extractSubdomain(hostname, domain); ok {
		subdomain = s
	} else {
		parts := strings.Split(hostname, ".")
		if len(parts) >= 1 {
			subdomain = parts[0]
		} else {
			subdomain = ""
		}
	}

	if namedSvc, ok := s.Config.ServicesBySubdomain[subdomain]; ok {
		return namedSvc, true
	} else {
		return service.NamedService{}, false
	}
}

func (s *Server) getOrCreateState(namedSvc service.NamedService) *service.ServiceState {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if state, ok := s.Services[namedSvc.Name]; ok {
		return state
	}

	state := &service.ServiceState{
		Name:     namedSvc.Name,
		Service:  namedSvc.Svc,
		EventBus: s.EventBus,
	}
	s.Services[namedSvc.Name] = state
	return state
}

func (s *Server) handleAltHost(w http.ResponseWriter, r *http.Request, ahName string, ah *althost.AltHost) {
	tunnel := ah.GetTunnel()
	if tunnel == nil {
		http.Error(w, "Alt host configured but no tunnel settings found", http.StatusInternalServerError)
		return
	}
	if err := tunnel.Open(); err != nil {
		log.Printf("Failed to open SSH tunnel for %s: %v", ahName, err)
		http.Error(w, "Failed to establish SSH tunnel", http.StatusBadGateway)
		return
	}
	tunnel.Forward(w, r)
}
