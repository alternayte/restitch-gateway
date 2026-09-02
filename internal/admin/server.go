package admin

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/alternayte/restitch-gateway/internal/ratelimit"
)

// Config holds admin server configuration.
type Config struct {
	Enabled        bool          `yaml:"enabled"`
	Port           int           `yaml:"port"`
	Bind           string        `yaml:"bind"`
	APIKey         string        `yaml:"api_key"`
	RequestLogSize int           `yaml:"request_log_size"`
	Storage        StorageConfig `yaml:"storage"`
}

// StorageConfig configures the time-series/request storage backend.
type StorageConfig struct {
	Type      string `yaml:"type"`
	URL       string `yaml:"url"`
	AuthToken string `yaml:"auth_token"`
	Retention string `yaml:"retention"`
}

// CompositionInfo describes a composition for the admin API.
type CompositionInfo struct {
	Name   string     `json:"name"`
	Path   string     `json:"path"`
	Method string     `json:"method"`
	Public bool       `json:"public"`
	Steps  []StepInfo `json:"steps"`
	Waves  [][]string `json:"waves"`
}

// StepInfo describes a step within a composition.
type StepInfo struct {
	Name         string   `json:"name"`
	Upstream     string   `json:"upstream"`
	Method       string   `json:"method"`
	Optional     bool     `json:"optional"`
	TimeoutMS    int64    `json:"timeout_ms"`
	DependsOn    []string `json:"depends_on"`
	InferredDeps []string `json:"inferred_deps"`
}

// UpstreamInfo describes an upstream for the admin API.
type UpstreamInfo struct {
	Name      string             `json:"name"`
	URL       string             `json:"url"`
	AuthType  string             `json:"auth_type"`
	TimeoutMS int64              `json:"timeout_ms"`
	Health    UpstreamHealthInfo `json:"health"`
}

// UpstreamHealthInfo describes the health status of an upstream.
type UpstreamHealthInfo struct {
	Status    string  `json:"status"`
	LatencyMS float64 `json:"latency_ms"`
	CheckedAt string  `json:"checked_at"`
	Error     string  `json:"error,omitempty"`
}

// Deps holds dependencies injected into the admin server.
type Deps struct {
	Version        string
	ConfigPath     string
	ConfigHash     func() string
	Compositions   func() []CompositionInfo
	Upstreams      func(ctx context.Context) []UpstreamInfo
	Requests       *RingBuffer
	Stats          *Stats
	Metrics        http.Handler
	Validate       func(yamlBytes []byte) []string
	Reload         func() (string, error)
	Storage        Storage
	RegistryStatus func() any // nil in file mode; returns registry poll status JSON
}

// Server is the admin API server.
type Server struct {
	cfg        Config
	deps       Deps
	httpServer *http.Server
	startTime  time.Time
}

// New creates an admin server.
func New(cfg Config, deps Deps) *Server {
	if cfg.Port == 0 {
		cfg.Port = 9090
	}
	if cfg.Bind == "" {
		cfg.Bind = "127.0.0.1"
	}
	if !isLoopback(cfg.Bind) {
		slog.Warn("admin server bound to a non-loopback address",
			"bind", cfg.Bind,
			"hint", "the admin API exposes request logs and config data; use this only in a trusted network")
	}
	s := &Server{cfg: cfg, deps: deps, startTime: time.Now()}

	mux := http.NewServeMux()

	// Rate limiter for mutation endpoints: 10 req/s, burst 5, per IP
	mutationLimiter := ratelimit.New(ratelimit.Config{
		RequestsPerSecond: 10,
		Burst:             5,
		Key:               "ip",
	})

	mux.HandleFunc("GET /admin/api/info", s.requireKey(s.handleInfo))
	mux.HandleFunc("GET /admin/api/compositions", s.requireKey(s.handleCompositions))
	mux.HandleFunc("GET /admin/api/compositions/{name}", s.requireKey(s.handleCompositionByName))
	mux.HandleFunc("GET /admin/api/upstreams", s.requireKey(s.handleUpstreams))
	mux.HandleFunc("GET /admin/api/stats/timeseries", s.requireKey(s.handleTimeSeries))
	mux.HandleFunc("GET /admin/api/stats/steps", s.requireKey(s.handleStepMetrics))
	mux.HandleFunc("GET /admin/api/requests/{id}", s.requireKey(s.handleRequestByID))
	mux.HandleFunc("GET /admin/api/requests", s.requireKey(s.handleRequests))
	mux.HandleFunc("GET /admin/api/stats", s.requireKey(s.handleStats))
	mux.HandleFunc("POST /admin/api/validate", s.requireKey(s.rateLimited(mutationLimiter, s.handleValidate)))
	mux.HandleFunc("POST /admin/api/reload", s.requireKey(s.rateLimited(mutationLimiter, s.handleReload)))
	if deps.RegistryStatus != nil {
		mux.HandleFunc("GET /admin/api/registry/status", s.requireKey(s.handleRegistryStatus))
	}
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	if deps.Metrics != nil {
		mux.Handle("GET /metrics", deps.Metrics)
	}

	s.httpServer = &http.Server{
		Addr:              net.JoinHostPort(cfg.Bind, strconv.Itoa(cfg.Port)),
		Handler:           corsMiddleware(mux, cfg.APIKey),
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return s
}

// isLoopback reports whether bind names the loopback interface.
func isLoopback(bind string) bool {
	switch bind {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// Start starts the admin server in the background.
func (s *Server) Start() error {
	slog.Info("admin server starting", "port", s.cfg.Port)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("admin server error", "error", err)
		}
	}()
	return nil
}

// Shutdown gracefully shuts down the admin server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// requireKey rejects requests without a valid X-Admin-Key. The key is
// required by default: with no configured key, no request can match and the
// admin API stays locked. This covers findings C3 and M9.
func (s *Server) requireKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.APIKey == "" || !keyMatches(r.Header.Get("X-Admin-Key"), s.cfg.APIKey) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// keyMatches reports whether got equals want in constant time when the
// lengths match. A length mismatch rejects immediately.
func keyMatches(got, want string) bool {
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func (s *Server) rateLimited(limiter *ratelimit.Limiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow(r) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
			return
		}
		next(w, r)
	}
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"version":        s.deps.Version,
		"uptime_seconds": time.Since(s.startTime).Seconds(),
		"config_hash":    s.deps.ConfigHash(),
		"config_path":    s.deps.ConfigPath,
	}
	if s.deps.Compositions != nil {
		resp["compositions"] = len(s.deps.Compositions())
	}
	if s.deps.Upstreams != nil {
		resp["upstreams"] = len(s.deps.Upstreams(r.Context()))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCompositions(w http.ResponseWriter, r *http.Request) {
	if s.deps.Compositions == nil {
		writeJSON(w, http.StatusOK, []CompositionInfo{})
		return
	}
	writeJSON(w, http.StatusOK, s.deps.Compositions())
}

func (s *Server) handleCompositionByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.deps.Compositions == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	for _, c := range s.deps.Compositions() {
		if c.Name == name {
			writeJSON(w, http.StatusOK, c)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (s *Server) handleUpstreams(w http.ResponseWriter, r *http.Request) {
	if s.deps.Upstreams == nil {
		writeJSON(w, http.StatusOK, []UpstreamInfo{})
		return
	}
	writeJSON(w, http.StatusOK, s.deps.Upstreams(r.Context()))
}

func (s *Server) handleRequests(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	if s.deps.Storage != nil {
		opts := RequestQuery{Limit: limit}
		opts.Composition = r.URL.Query().Get("composition")
		if v := r.URL.Query().Get("status_min"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				opts.StatusMin = n
			}
		}
		if v := r.URL.Query().Get("status_max"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				opts.StatusMax = n
			}
		}
		if v := r.URL.Query().Get("min_duration_ms"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				opts.MinDuration = f
			}
		}
		if v := r.URL.Query().Get("partial"); v != "" {
			p := v == "true"
			opts.Partial = &p
		}
		records, err := s.deps.Storage.QueryRequests(r.Context(), opts)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, records)
		return
	}

	writeJSON(w, http.StatusOK, s.deps.Requests.List(limit))
}

func (s *Server) handleTimeSeries(w http.ResponseWriter, r *http.Request) {
	if s.deps.Storage == nil {
		writeJSON(w, http.StatusOK, []Bucket{})
		return
	}

	rangeDur := ParseRangeDuration(r.URL.Query().Get("range"), time.Hour)
	resolution := ParseRangeDuration(r.URL.Query().Get("resolution"), time.Minute)
	composition := r.URL.Query().Get("composition")

	to := time.Now()
	from := to.Add(-rangeDur)

	buckets, err := s.deps.Storage.QueryTimeSeries(r.Context(), from, to, resolution, composition)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if buckets == nil {
		buckets = []Bucket{}
	}
	writeJSON(w, http.StatusOK, buckets)
}

func (s *Server) handleStepMetrics(w http.ResponseWriter, r *http.Request) {
	if s.deps.Storage == nil {
		writeJSON(w, http.StatusOK, []StepAggregate{})
		return
	}

	composition := r.URL.Query().Get("composition")
	if composition == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "composition parameter required"})
		return
	}

	rangeDur := ParseRangeDuration(r.URL.Query().Get("range"), time.Hour)
	to := time.Now()
	from := to.Add(-rangeDur)

	metrics, err := s.deps.Storage.QueryStepMetrics(r.Context(), composition, from, to)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if metrics == nil {
		metrics = []StepAggregate{}
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (s *Server) handleRequestByID(w http.ResponseWriter, r *http.Request) {
	if s.deps.Storage == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "storage not configured"})
		return
	}

	id := r.PathValue("id")
	rec, err := s.deps.Storage.GetRequestByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if rec == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "request not found"})
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// ParseRangeDuration parses a range/resolution shorthand string (e.g. "1h",
// "5m", "7d") into a time.Duration, returning fallback if s is unrecognized.
func ParseRangeDuration(s string, fallback time.Duration) time.Duration {
	switch s {
	case "1h":
		return time.Hour
	case "6h":
		return 6 * time.Hour
	case "24h":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	case "1m":
		return time.Minute
	case "5m":
		return 5 * time.Minute
	default:
		return fallback
	}
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.deps.Stats.Snapshot())
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"valid":  false,
			"errors": []string{"failed to read request body"},
		})
		return
	}
	errs := s.deps.Validate(body)
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":  len(errs) == 0,
		"errors": errs,
	})
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	hash, err := s.deps.Reload()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":     false,
			"errors": []string{err.Error()},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"config_hash": hash,
	})
}

func (s *Server) handleRegistryStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.deps.RegistryStatus())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// corsMiddleware adds CORS headers and answers preflight OPTIONS requests.
// It requires a valid key on OPTIONS too, so a cross-site page cannot use a
// keyless preflight to learn the API's capabilities or trigger mutations
// (finding C4). Because the key is required on every request, reflecting the
// Origin is safe: only authenticated clients receive CORS headers.
func corsMiddleware(next http.Handler, apiKey string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions && (apiKey == "" || !keyMatches(r.Header.Get("X-Admin-Key"), apiKey)) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
