package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/argus-sec/argus/internal/agent"
	"github.com/argus-sec/argus/internal/sse"
)

// --- Health ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "argus",
		"version": "0.2.0",
	})
}

// --- Scan (stateless) ---

type scanRequest struct {
	TargetPath string `json:"target_path"`
	Role       string `json:"role"` // "recon" or "exploit", defaults to "recon"
}

type scanResponse struct {
	SessionID string `json:"session_id"`
}

func (s *Server) handleStartScan(w http.ResponseWriter, r *http.Request) {
	var req scanRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.TargetPath == "" {
		writeError(w, http.StatusBadRequest, "target_path is required")
		return
	}

	// Default role to recon.
	if req.Role == "" {
		req.Role = "recon"
	}
	if req.Role != "recon" && req.Role != "exploit" {
		writeError(w, http.StatusBadRequest, "role must be 'recon' or 'exploit'")
		return
	}

	// Validate that the target path exists and is a directory.
	info, err := os.Stat(req.TargetPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("target_path %q does not exist: %v", req.TargetPath, err))
		return
	}
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("target_path %q is not a directory", req.TargetPath))
		return
	}

	// Resolve the LLM provider.
	provider, ok := s.providers[req.Role]
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("no LLM provider configured for role %q", req.Role))
		return
	}

	// Generate an in-memory session ID.
	sessionID := generateID()

	// Create sandbox.
	sandbox, err := agent.NewSandbox(req.TargetPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create sandbox: %v", err))
		return
	}

	// Get the SSE channel and start the agent runner.
	eventCh := s.broker.GetOrCreateChannel(sessionID)
	runner := agent.NewRunner(sandbox, provider, eventCh)

	go func() {
		if err := runner.Run(context.Background(), sessionID, req.Role); err != nil {
			log.Printf("[agent] session %s failed: %v", sessionID, err)
			s.broker.Publish(sessionID, sse.Event{
				Event: "error",
				Data:  fmt.Sprintf(`{"error":%q}`, err.Error()),
			})
		}
	}()

	writeJSON(w, http.StatusCreated, scanResponse{SessionID: sessionID})
}

// --- SSE Stream ---

func (s *Server) handleSessionStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	ch := s.broker.Subscribe(id)

	// Send initial connection event.
	fmt.Fprintf(w, "event: connected\ndata: {\"session_id\":%q}\n\n", id)
	flusher.Flush()

	// Stream events until client disconnects or session ends.
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, open := <-ch:
			if !open {
				fmt.Fprintf(w, "event: session_end\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Event, event.Data)
			flusher.Flush()
		}
	}
}

// --- Helpers ---

func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("failed to generate random ID: %v", err))
	}
	return hex.EncodeToString(b)
}
