package main

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gin-contrib/sse"
)

type Event struct {
	Type    string `json:"type"`
	Service string `json:"service"`
}

type EventBus struct {
	mu      sync.RWMutex
	counter int64
	events  map[int64]chan<- Event
}

func NewEventBus() *EventBus {
	return &EventBus{
		events: make(map[int64]chan<- Event),
	}
}

func (eb *EventBus) Publish(e Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	for _, ch := range eb.events {
		// Use a non-blocking send to avoid blocking if a receiver is slow
		select {
		case ch <- e:
		default:
			// If channel is full, skip this receiver
		}
	}
}

func (eb *EventBus) Subscribe() (int64, <-chan Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	id := eb.counter
	eb.counter += 1
	ch := make(chan Event, 10) // Buffered channel to prevent blocking
	eb.events[id] = ch
	return id, ch
}

func (eb *EventBus) Unsubscribe(id int64) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	if ch, ok := eb.events[id]; ok {
		close(ch)
		delete(eb.events, id)
	}
}

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
		}
	}
}
