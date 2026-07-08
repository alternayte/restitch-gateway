package server

import (
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// Config holds server configuration options.
type Config struct {
	Port    int
	TLSPort int
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

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", config.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

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

// ListenAndServe starts the HTTP server. Ready is set only after the
// listener is bound, so /ready never returns 200 before the port is open.
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	s.ready.Store(true)
	return s.httpServer.Serve(ln)
}

// ListenAndServeTLS starts the HTTPS server with TLS.
func (s *Server) ListenAndServeTLS(certFile, keyFile string) error {
	tlsConfig, err := LoadTLSConfig(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("failed to load TLS config: %w", err)
	}

	s.httpsServer.TLSConfig = tlsConfig

	ln, err := net.Listen("tcp", s.httpsServer.Addr)
	if err != nil {
		return err
	}
	s.ready.Store(true)
	return s.httpsServer.ServeTLS(ln, "", "")
}

// Ready returns whether the server is ready to accept requests.
func (s *Server) Ready() bool {
	return s.ready.Load()
}

// SetReady sets the server's ready state.
func (s *Server) SetReady(ready bool) {
	s.ready.Store(ready)
}

// StartTime returns the time when the server was created.
func (s *Server) StartTime() time.Time {
	return s.startTime
}
