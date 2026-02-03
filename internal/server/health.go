package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"sync"
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

// UpstreamHealthResponse represents the response for /health/upstreams.
type UpstreamHealthResponse struct {
	Status    string                    `json:"status"` // "healthy" if all upstreams OK, "degraded" if any unhealthy
	Upstreams map[string]UpstreamStatus `json:"upstreams"`
}

// UpstreamStatus represents the health status of a single upstream.
type UpstreamStatus struct {
	URL       string  `json:"url"`
	Status    string  `json:"status"`          // "healthy" or "unhealthy"
	LatencyMS float64 `json:"latency_ms"`      // Response time in milliseconds
	LastCheck string  `json:"last_check"`      // ISO8601 timestamp
	Error     string  `json:"error,omitempty"` // Error message if unhealthy
}

// UpstreamInfo contains the upstream configuration needed for health checks.
// This avoids import cycles with the composition package.
type UpstreamInfo struct {
	URL        string
	HealthPath string
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

// checkUpstreamHealth performs a health check on a single upstream.
// Uses HEAD request to base URL by default, or GET to configured health_path if set.
func checkUpstreamHealth(ctx context.Context, name string, upstream UpstreamInfo, httpClient *http.Client) UpstreamStatus {
	start := time.Now()

	// Build health check URL
	healthURL := upstream.URL
	if upstream.HealthPath != "" {
		healthURL = strings.TrimSuffix(upstream.URL, "/") + upstream.HealthPath
	}

	// Create HEAD request (or GET if health_path is set)
	method := http.MethodHead
	if upstream.HealthPath != "" {
		method = http.MethodGet
	}

	req, err := http.NewRequestWithContext(ctx, method, healthURL, nil)
	if err != nil {
		return UpstreamStatus{
			URL:       upstream.URL,
			Status:    "unhealthy",
			LatencyMS: float64(time.Since(start).Milliseconds()),
			LastCheck: time.Now().UTC().Format(time.RFC3339),
			Error:     err.Error(),
		}
	}

	resp, err := httpClient.Do(req)
	latency := float64(time.Since(start).Nanoseconds()) / 1e6

	if err != nil {
		return UpstreamStatus{
			URL:       upstream.URL,
			Status:    "unhealthy",
			LatencyMS: latency,
			LastCheck: time.Now().UTC().Format(time.RFC3339),
			Error:     err.Error(),
		}
	}
	defer resp.Body.Close()

	// Consider 2xx and 3xx as healthy
	status := "healthy"
	var errMsg string
	if resp.StatusCode >= 400 {
		status = "unhealthy"
		errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	return UpstreamStatus{
		URL:       upstream.URL,
		Status:    status,
		LatencyMS: latency,
		LastCheck: time.Now().UTC().Format(time.RFC3339),
		Error:     errMsg,
	}
}

// UpstreamHealthHandler creates an HTTP handler for the /health/upstreams endpoint.
// Returns status of all configured upstreams with latency and last check time.
// The httpClient should be the Phase 1 client with optimized connection pooling.
func UpstreamHealthHandler(upstreamInfos map[string]UpstreamInfo, httpClient *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		upstreams := make(map[string]UpstreamStatus)
		overallStatus := "healthy"

		// Check all upstreams concurrently
		var wg sync.WaitGroup
		var mu sync.Mutex

		for name, info := range upstreamInfos {
			name := name
			upstream := info
			wg.Add(1)
			go func() {
				defer wg.Done()
				status := checkUpstreamHealth(ctx, name, upstream, httpClient)

				mu.Lock()
				upstreams[name] = status
				if status.Status == "unhealthy" {
					overallStatus = "degraded"
				}
				mu.Unlock()
			}()
		}

		wg.Wait()

		response := UpstreamHealthResponse{
			Status:    overallStatus,
			Upstreams: upstreams,
		}

		w.Header().Set("Content-Type", "application/json")
		// Return 200 even if some upstreams are unhealthy (for monitoring tools)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}
