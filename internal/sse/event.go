// Package sse defines shared types for Server-Sent Events,
// decoupling the api and agent packages to avoid import cycles.
package sse

// Event is a structured SSE event pushed to clients.
type Event struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}
