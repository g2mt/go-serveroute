package event

import (
	"sync"
)

type Event struct {
	Type    string `json:"type"`    // either "start" or "stop"
	Service string `json:"service"` // name of service
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

func (eb *EventBus) Close() {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	for _, ch := range eb.events {
		close(ch)
	}
	eb.events = nil
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
