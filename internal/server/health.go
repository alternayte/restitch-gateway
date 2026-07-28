package server

import (
	"encoding/json"
	"net/http"
)

// Version is set at build time via ldflags.
var Version = "dev"

// HealthResponse represents the JSON response for the /health endpoint.
type HealthResponse struct {
	Status string `json:"status"`
}

// ReadyResponse represents the JSON response for the /ready endpoint.
type ReadyResponse struct {
	Status string `json:"status"`
}

// HealthHandler creates an HTTP handler for the /health endpoint.
func HealthHandler(srv *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(HealthResponse{Status: "ok"})
	}
}

// ReadyHandler creates an HTTP handler for the /ready endpoint.
func ReadyHandler(srv *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if srv.Ready() {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(ReadyResponse{Status: "ready"})
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(ReadyResponse{Status: "not_ready"})
		}
	}
}
