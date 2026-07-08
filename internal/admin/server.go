package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// Config holds admin server configuration.
type Config struct {
	Enabled        bool   `yaml:"enabled"`
	Port           int    `yaml:"port"`
	APIKey         string `yaml:"api_key"`
	RequestLogSize int    `yaml:"request_log_size"`
}

// Deps holds dependencies injected into the admin server.
type Deps struct {
	Version    string
	ConfigPath string
	ConfigHash func() string
	Requests   *RingBuffer
	Stats      *Stats
	Metrics    http.Handler
	Validate   func(yamlBytes []byte) []string
	Reload     func() (string, error)
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
	s := &Server{cfg: cfg, deps: deps, startTime: time.Now()}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /admin/api/info", s.requireKey(s.handleInfo))
	mux.HandleFunc("GET /admin/api/requests", s.requireKey(s.handleRequests))
	mux.HandleFunc("GET /admin/api/stats", s.requireKey(s.handleStats))
	mux.HandleFunc("POST /admin/api/validate", s.requireKey(s.handleValidate))
	mux.HandleFunc("POST /admin/api/reload", s.requireKey(s.handleReload))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	if deps.Metrics != nil {
		mux.Handle("GET /metrics", deps.Metrics)
	}

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: corsMiddleware(mux),
	}

	return s
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

func (s *Server) requireKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.APIKey != "" {
			if r.Header.Get("X-Admin-Key") != s.cfg.APIKey {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":        s.deps.Version,
		"uptime_seconds": time.Since(s.startTime).Seconds(),
		"config_hash":    s.deps.ConfigHash(),
		"config_path":    s.deps.ConfigPath,
	})
}

func (s *Server) handleRequests(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	writeJSON(w, http.StatusOK, s.deps.Requests.List(limit))
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.deps.Stats.Snapshot())
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var buf [1 << 20]byte
	n, _ := r.Body.Read(buf[:])
	errs := s.deps.Validate(buf[:n])
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Key")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
