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
	config     Config
	router     *Router
	httpServer *http.Server
	ready      atomic.Bool
}

// New creates a new Server with the given configuration.
func New(config Config) *Server {
	router := NewRouter()

	s := &Server{
		config: config,
		router: router,
	}

	// Create HTTP server with proper timeouts
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", config.Port),
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

// Ready returns whether the server is ready to accept requests.
func (s *Server) Ready() bool {
	return s.ready.Load()
}
