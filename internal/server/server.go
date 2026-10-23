// Copyright 2026 Restitch maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	// ReadTimeout and WriteTimeout default to 30s when zero.
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// Server represents the restitch HTTP/HTTPS server.
type Server struct {
	config      Config
	router      *Router
	httpServer  *http.Server
	httpsServer *http.Server
	ready       atomic.Bool
	// unconfigured holds /ready at 503 until the first artifact is applied
	// (managed mode). It is false by default, so standalone mode is ready
	// once the listener is bound.
	unconfigured atomic.Bool
	startTime   time.Time
}

// New creates a new Server with the given configuration.
func New(config Config) *Server {
	router := NewRouter()

	if config.ReadTimeout == 0 {
		config.ReadTimeout = 30 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 30 * time.Second
	}
	s := &Server{
		config:    config,
		router:    router,
		startTime: time.Now(),
	}

	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", config.Port),
		Handler:           router,
		ReadTimeout:       config.ReadTimeout,
		WriteTimeout:      config.WriteTimeout,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	s.httpsServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", config.TLSPort),
		Handler:           router,
		ReadTimeout:       config.ReadTimeout,
		WriteTimeout:      config.WriteTimeout,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return s
}

// Router returns the server's router for registering handlers.
func (s *Server) Router() *Router {
	return s.router
}

// SetHandler overrides the server's handler (used when wrapping with middleware).
func (s *Server) SetHandler(h http.Handler) {
	s.httpServer.Handler = h
	s.httpsServer.Handler = h
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

// Ready returns whether the server is ready to accept requests: the
// listener is bound and a config is applied.
func (s *Server) Ready() bool {
	return s.ready.Load() && !s.unconfigured.Load()
}

// SetConfigured records whether a config is applied. A managed gateway
// starts unconfigured and turns configured after its first pull.
func (s *Server) SetConfigured(configured bool) {
	s.unconfigured.Store(!configured)
}

// SetReady sets the server's ready state.
func (s *Server) SetReady(ready bool) {
	s.ready.Store(ready)
}

// StartTime returns the time when the server was created.
func (s *Server) StartTime() time.Time {
	return s.startTime
}
