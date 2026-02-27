package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"serveroute/internal/service"
)

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request, state *service.ServiceState) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")

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
	s.Mu.Lock()
	defer s.Mu.Unlock()

	result := make(map[string]interface{})

	for name, svc := range s.Config.Services {
		if svc.Hidden {
			continue
		}

		status := "stopped"
		if state, ok := s.Services[name]; ok {
			if state.IsRunning() {
				status = "started"
			}
		}

		result[name] = map[string]interface{}{
			"status":    status,
			"subdomain": svc.Subdomain,
		}
	}

	json.NewEncoder(w).Encode(result)
}
