// Package api implements the HTTP API server and SSE streaming for Argus.
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/argus-sec/argus/internal/llm"
	"github.com/argus-sec/argus/internal/sse"
)

// Server is the main HTTP API server for Argus.
// Fully stateless — no database, all state lives in memory.
type Server struct {
	mux       *http.ServeMux
	providers map[string]llm.Provider // keyed by role: "recon", "exploit"
	broker    *SSEBroker
}

// NewServer creates a new API server with all routes registered.
func NewServer(providers map[string]llm.Provider) *Server {
	s := &Server{
		mux:       http.NewServeMux(),
		providers: providers,
		broker:    NewSSEBroker(),
	}
	s.registerRoutes()
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Apply CORS middleware for local frontend development.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	s.mux.ServeHTTP(w, r)
}

// registerRoutes sets up all API endpoints.
func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("POST /api/scan", s.handleStartScan)
	s.mux.HandleFunc("GET /api/sessions/{id}/stream", s.handleSessionStream)
}

// --- SSE Broker ---

// SSEEvent is an alias for the shared sse.Event type.
type SSEEvent = sse.Event

// SSEBroker manages per-session SSE channels with deadlock prevention.
type SSEBroker struct {
	mu       sync.RWMutex
	channels map[string]chan sse.Event
}

// NewSSEBroker creates a new SSE broker.
func NewSSEBroker() *SSEBroker {
	return &SSEBroker{
		channels: make(map[string]chan sse.Event),
	}
}

// GetOrCreateChannel returns the event channel for a session, creating it if needed.
func (b *SSEBroker) GetOrCreateChannel(sessionID string) chan sse.Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.channels[sessionID]; ok {
		return ch
	}

	ch := make(chan sse.Event, 256)
	b.channels[sessionID] = ch
	return ch
}

// Publish sends an event to the session's channel without blocking.
func (b *SSEBroker) Publish(sessionID string, event sse.Event) {
	b.mu.RLock()
	ch, ok := b.channels[sessionID]
	b.mu.RUnlock()

	if !ok {
		return
	}

	// Non-blocking send with drain-on-full to prevent agent deadlock.
	select {
	case ch <- event:
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- event:
		default:
			log.Printf("[sse] dropping event for session %s: channel full", sessionID)
		}
	}
}

// Subscribe returns the channel for a client to read from.
func (b *SSEBroker) Subscribe(sessionID string) <-chan sse.Event {
	return b.GetOrCreateChannel(sessionID)
}

// CloseChannel removes and closes the channel for a session.
func (b *SSEBroker) CloseChannel(sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.channels[sessionID]; ok {
		close(ch)
		delete(b.channels, sessionID)
	}
}

// --- JSON helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[api] error encoding JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}
