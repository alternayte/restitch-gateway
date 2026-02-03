package server

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// Config holds server configuration options.
type Config struct {
	Port      int    // HTTP server port
	TLSPort   int    // HTTPS server port
	LogFormat string // Log format: "json" or "text"
}

// Server represents the restitch HTTP/HTTPS server.
type Server struct {
	config      Config
	router      *Router
	httpServer  *http.Server
	httpsServer *http.Server
	ready       atomic.Bool
	startTime   time.Time
}

// New creates a new Server with the given configuration.
func New(config Config) *Server {
	router := NewRouter()

	s := &Server{
		config:    config,
		router:    router,
		startTime: time.Now(),
	}

	// Create HTTP server with proper timeouts
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", config.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Create HTTPS server with same timeouts (TLS config added in ListenAndServeTLS)
	s.httpsServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", config.TLSPort),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return s
}

// Router returns the server's router for registering handlers.
func (s *Server) Router() *Router {
	return s.router
}

// ListenAndServe starts the HTTP server.
// It blocks until the server is stopped or an error occurs.
func (s *Server) ListenAndServe() error {
	s.ready.Store(true)
	return s.httpServer.ListenAndServe()
}

// ListenAndServeTLS starts the HTTPS server with TLS.
// It loads the TLS configuration from the provided certificate and key files.
// It blocks until the server is stopped or an error occurs.
func (s *Server) ListenAndServeTLS(certFile, keyFile string) error {
	tlsConfig, err := LoadTLSConfig(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("failed to load TLS config: %w", err)
	}

	s.httpsServer.TLSConfig = tlsConfig
	s.ready.Store(true)

	// ListenAndServeTLS with empty cert/key since we already configured TLSConfig
	return s.httpsServer.ListenAndServeTLS("", "")
}

// Ready returns whether the server is ready to accept requests.
func (s *Server) Ready() bool {
	return s.ready.Load()
}

// SetReady sets the server's ready state.
// Used during shutdown to indicate the server is no longer accepting traffic.
func (s *Server) SetReady(ready bool) {
	s.ready.Store(ready)
}

// StartTime returns the time when the server was created.
func (s *Server) StartTime() time.Time {
	return s.startTime
}
