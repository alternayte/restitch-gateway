package server

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"
)

// Version is set at build time via ldflags.
// Default to "dev" for development builds.
var Version = "dev"

// HealthResponse represents the JSON response for the /health endpoint.
type HealthResponse struct {
	Status  string         `json:"status"`
	Uptime  string         `json:"uptime"`
	Version string         `json:"version"`
	Memory  MemoryResponse `json:"memory"`
}

// MemoryResponse represents memory statistics in the health response.
type MemoryResponse struct {
	AllocMB uint64 `json:"alloc_mb"`
	SysMB   uint64 `json:"sys_mb"`
}

// ReadyResponse represents the JSON response for the /ready endpoint.
type ReadyResponse struct {
	Status string `json:"status"`
}

// HealthHandler creates an HTTP handler for the /health endpoint.
// Returns 200 OK with JSON body containing status, uptime, version, and memory usage.
func HealthHandler(srv *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Calculate uptime
		uptime := time.Since(srv.StartTime()).Round(time.Second).String()

		// Get memory statistics
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		response := HealthResponse{
			Status:  "healthy",
			Uptime:  uptime,
			Version: Version,
			Memory: MemoryResponse{
				AllocMB: memStats.Alloc / (1024 * 1024),
				SysMB:   memStats.Sys / (1024 * 1024),
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// ReadyHandler creates an HTTP handler for the /ready endpoint.
// Returns 200 OK when the server can accept traffic, 503 when shutting down.
func ReadyHandler(srv *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if srv.Ready() {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(ReadyResponse{Status: "ready"})
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(ReadyResponse{Status: "not_ready"})
		}
	}
}
