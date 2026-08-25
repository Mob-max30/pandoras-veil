package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type StreamMessage struct {
	ID         string `json:"id"`
	Ciphertext string `json:"ciphertext"`
	Sender     string `json:"sender,omitempty"`
}

// StreamBroker manages real-time SSE subscriptions by recipient handle
type StreamBroker struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan StreamMessage]struct{}
}

func NewStreamBroker() *StreamBroker {
	return &StreamBroker{
		subscribers: make(map[string]map[chan StreamMessage]struct{}),
	}
}

// Subscribe registers a new subscriber channel for a handle
func (b *StreamBroker) Subscribe(handle string) chan StreamMessage {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan StreamMessage, 10)
	if _, exists := b.subscribers[handle]; !exists {
		b.subscribers[handle] = make(map[chan StreamMessage]struct{})
	}
	b.subscribers[handle][ch] = struct{}{}
	return ch
}

// Unsubscribe removes a channel
func (b *StreamBroker) Unsubscribe(handle string, ch chan StreamMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if subs, exists := b.subscribers[handle]; exists {
		delete(subs, ch)
		close(ch)
		if len(subs) == 0 {
			delete(b.subscribers, handle)
		}
	}
}

// Broadcast sends an encrypted message event to all active streams for a recipient
func (b *StreamBroker) Broadcast(recipientHandle string, id, ciphertext, sender string) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	subs, exists := b.subscribers[recipientHandle]
	if !exists {
		return
	}

	msg := StreamMessage{
		ID:         id,
		Ciphertext: ciphertext,
		Sender:     sender,
	}

	for ch := range subs {
		select {
		case ch <- msg:
		default:
			// Buffer full, skip to avoid blocking
		}
	}
}

// HandleStream handles GET /stream?handle=<handle> (SSE)
func (b *StreamBroker) HandleStream(w http.ResponseWriter, r *http.Request) {
	handle := r.URL.Query().Get("handle")
	if handle == "" {
		http.Error(w, "handle query parameter is required", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Send initial connection heartbeat
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	msgChan := b.Subscribe(handle)
	defer b.Unsubscribe(handle, msgChan)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgChan:
			if !ok {
				return
			}
			data, err := json.Marshal(msg)
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	}
}
