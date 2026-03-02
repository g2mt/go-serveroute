package server

import (
	"encoding/json"
	"net/http"

	"github.com/gin-contrib/sse"
)

func (s *Server) apiEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", sse.ContentType)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	id, ch := s.EventBus.Subscribe()
	defer s.EventBus.Unsubscribe(id)

	sse.Encode(w, sse.Event{
		Event: "connected",
		Data:  "connected",
	})

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Listen for events and send them to client
	for {
		select {
		case event := <-ch:
			data, err := json.Marshal(event)
			if err != nil {
				return
			}

			if err := sse.Encode(w, sse.Event{
				Event: "message",
				Data:  string(data),
			}); err != nil {
				return
			}

			// Flush to ensure the event is sent immediately
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		case <-r.Context().Done():
			// Client disconnected
			return
		default:
			return
		}
	}
}
