package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

type streamClient struct {
	send chan []byte
}

// StreamHub coordinates real-time SSE streaming connections to clients.
type StreamHub struct {
	mu         sync.RWMutex
	clients    map[*streamClient]bool
	register   chan *streamClient
	unregister chan *streamClient
	broadcast  chan []byte
	stopCh     chan struct{}
}

// NewStreamHub creates a new SSE StreamHub.
func NewStreamHub() *StreamHub {
	return &StreamHub{
		clients:    make(map[*streamClient]bool),
		register:   make(chan *streamClient),
		unregister: make(chan *streamClient),
		broadcast:  make(chan []byte, 100),
		stopCh:     make(chan struct{}),
	}
}

// Run starts the StreamHub event loop.
func (h *StreamHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Drop or slow client
				}
			}
			h.mu.RUnlock()

		case <-h.stopCh:
			h.mu.Lock()
			for client := range h.clients {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			return
		}
	}
}

// Broadcast sends a custom event and payload to all connected SSE clients.
func (h *StreamHub) Broadcast(event string, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(data))
	select {
	case h.broadcast <- []byte(msg):
	default:
	}
}

// BroadcastFlags serializes and broadcasts updated flags to all connected SSE clients.
func (h *StreamHub) BroadcastFlags(flags []domain.FeatureFlag) {
	payload, err := json.Marshal(map[string]interface{}{
		"event":     "flags_update",
		"timestamp": time.Now().UTC().UnixMilli(),
		"flags":     flags,
	})
	if err != nil {
		return
	}

	msg := fmt.Sprintf("event: flags_update\ndata: %s\n\n", string(payload))
	select {
	case h.broadcast <- []byte(msg):
	default:
	}
}

// Close stops the StreamHub.
func (h *StreamHub) Close() {
	if h.stopCh != nil {
		select {
		case <-h.stopCh:
		default:
			close(h.stopCh)
		}
	}
}

// ActiveConnections returns the number of active streaming clients.
func (h *StreamHub) ActiveConnections() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// handleFlagsStream handles persistent HTTP Server-Sent Events (SSE) connections.
func (s *Server) handleFlagsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported by client connection", http.StatusBadRequest)
		return
	}

	// Set standard SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Send initial flags snapshot immediately on connection
	ctx := r.Context()
	projectID := s.resolveProjectID(r)
	flags, err := s.store.ListFlagsByProject(ctx, projectID)
	if err == nil {
		initPayload, _ := json.Marshal(map[string]interface{}{
			"event":     "flags_init",
			"timestamp": time.Now().UTC().UnixMilli(),
			"flags":     flags,
		})
		_, _ = fmt.Fprintf(w, "event: flags_init\ndata: %s\n\n", string(initPayload))
		flusher.Flush()
	}

	client := &streamClient{
		send: make(chan []byte, 20),
	}

	s.streamHub.register <- client
	defer func() {
		s.streamHub.unregister <- client
	}()

	// Ping keepalive ticker every 15s to prevent proxy/firewall disconnects
	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-pingTicker.C:
			_, err := fmt.Fprintf(w, ": ping\n\n")
			if err != nil {
				return
			}
			flusher.Flush()

		case msg, ok := <-client.send:
			if !ok {
				return
			}
			_, err := w.Write(msg)
			if err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) broadcastCurrentFlags(ctx context.Context) {
	if s.streamHub == nil {
		return
	}
	flags, err := s.store.ListFlags(ctx)
	if err == nil {
		s.streamHub.BroadcastFlags(flags)
	}
}
