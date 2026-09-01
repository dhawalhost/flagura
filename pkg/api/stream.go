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
	projectID   string
	environment string
	send        chan []byte
}

type streamBroadcastMsg struct {
	projectID   string
	environment string
	data        []byte
}

// StreamHub coordinates real-time SSE streaming connections isolated by tenant project and environment.
type StreamHub struct {
	mu         sync.RWMutex
	clients    map[*streamClient]bool
	subs       map[string]map[*streamClient]bool // key: projectID or projectID:env
	register   chan *streamClient
	unregister chan *streamClient
	broadcast  chan streamBroadcastMsg
	stopCh     chan struct{}
}

// NewStreamHub creates a new tenant-aware SSE StreamHub.
func NewStreamHub() *StreamHub {
	return &StreamHub{
		clients:    make(map[*streamClient]bool),
		subs:       make(map[string]map[*streamClient]bool),
		register:   make(chan *streamClient),
		unregister: make(chan *streamClient),
		broadcast:  make(chan streamBroadcastMsg, 100),
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
			keyProj := client.projectID
			if h.subs[keyProj] == nil {
				h.subs[keyProj] = make(map[*streamClient]bool)
			}
			h.subs[keyProj][client] = true

			if client.environment != "" {
				keyEnv := client.projectID + ":" + client.environment
				if h.subs[keyEnv] == nil {
					h.subs[keyEnv] = make(map[*streamClient]bool)
				}
				h.subs[keyEnv][client] = true
			}
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				if sub, ok := h.subs[client.projectID]; ok {
					delete(sub, client)
				}
				if client.environment != "" {
					if sub, ok := h.subs[client.projectID+":"+client.environment]; ok {
						delete(sub, client)
					}
				}
				close(client.send)
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			h.mu.RLock()
			targets := make(map[*streamClient]bool)

			// Deliver to all subscribers of this project
			if sub, ok := h.subs[msg.projectID]; ok {
				for c := range sub {
					// If client specified environment, match it
					if c.environment == "" || msg.environment == "" || c.environment == msg.environment {
						targets[c] = true
					}
				}
			}

			for client := range targets {
				select {
				case client.send <- msg.data:
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
			h.subs = make(map[string]map[*streamClient]bool)
			h.mu.Unlock()
			return
		}
	}
}

// BroadcastProjectFlags serializes and delivers flag updates strictly to authorized subscribers of projectID.
func (h *StreamHub) BroadcastProjectFlags(projectID string, env domain.Environment, configVersion uint64, flags []domain.FeatureFlag) {
	if projectID == "" {
		projectID = domain.DefaultProjectID
	}
	payload, err := json.Marshal(map[string]interface{}{
		"event":          "flags_update",
		"project_id":     projectID,
		"environment":    string(env),
		"config_version": configVersion,
		"timestamp":      time.Now().UTC().UnixMilli(),
		"flags":          flags,
	})
	if err != nil {
		return
	}

	msg := fmt.Sprintf("event: flags_update\ndata: %s\n\n", string(payload))
	select {
	case h.broadcast <- streamBroadcastMsg{projectID: projectID, environment: string(env), data: []byte(msg)}:
	default:
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
	case h.broadcast <- streamBroadcastMsg{projectID: domain.DefaultProjectID, data: []byte(msg)}:
	default:
	}
}

// BroadcastFlags is a backward-compatible wrapper that broadcasts to the default project.
func (h *StreamHub) BroadcastFlags(flags []domain.FeatureFlag) {
	h.BroadcastProjectFlags(domain.DefaultProjectID, "", 1, flags)
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

	ctx := r.Context()
	projectID := s.resolveProjectID(r)
	if projectID == "" {
		http.Error(w, "project_id is required via X-Project-ID header, project_id query parameter, or API key", http.StatusBadRequest)
		return
	}
	envParam := r.URL.Query().Get("environment")

	flags, err := s.store.ListFlagsByProject(ctx, projectID)
	if err == nil {
		var maxVersion uint64 = 1
		for _, f := range flags {
			if f.ConfigVersion > maxVersion {
				maxVersion = f.ConfigVersion
			}
		}
		initPayload, _ := json.Marshal(map[string]interface{}{
			"event":          "flags_init",
			"project_id":     projectID,
			"environment":    envParam,
			"config_version": maxVersion,
			"timestamp":      time.Now().UTC().UnixMilli(),
			"flags":          flags,
		})
		_, _ = fmt.Fprintf(w, "event: flags_init\ndata: %s\n\n", string(initPayload))
		flusher.Flush()
	}

	client := &streamClient{
		projectID:   projectID,
		environment: envParam,
		send:        make(chan []byte, 20),
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

func (s *Server) broadcastCurrentFlags(ctx context.Context, projectID string, env domain.Environment) {
	if s.streamHub == nil {
		return
	}
	if projectID == "" {
		projectID = domain.DefaultProjectID
	}
	flags, err := s.store.ListFlagsByProject(ctx, projectID)
	if err == nil {
		var maxVersion uint64 = 1
		for _, f := range flags {
			if f.ConfigVersion > maxVersion {
				maxVersion = f.ConfigVersion
			}
		}
		s.streamHub.BroadcastProjectFlags(projectID, env, maxVersion, flags)
	}
}
