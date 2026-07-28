package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// ShutdownTimeout is the maximum time to wait for in-flight requests to drain.
const ShutdownTimeout = 30 * time.Second

// WaitForShutdownSignal returns a channel that receives when a shutdown signal
// is received (SIGTERM or SIGINT). This allows the caller to handle shutdown
// in a select statement alongside other channels.
func (s *Server) WaitForShutdownSignal() <-chan struct{} {
	done := make(chan struct{})

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-quit
		fmt.Printf("shutdown signal received (%s), draining connections...\n", sig)
		s.SetReady(false)
		close(done)
	}()

	return done
}

// Shutdown gracefully shuts down both HTTP and HTTPS servers.
// It waits for in-flight requests to complete or until the context is cancelled.
func (s *Server) Shutdown(ctx context.Context) error {
	var httpErr, httpsErr error

	if s.httpServer != nil {
		httpErr = s.httpServer.Shutdown(ctx)
	}

	if s.httpsServer != nil {
		httpsErr = s.httpsServer.Shutdown(ctx)
	}

	if httpErr != nil {
		return fmt.Errorf("HTTP server shutdown: %w", httpErr)
	}
	if httpsErr != nil {
		return fmt.Errorf("HTTPS server shutdown: %w", httpsErr)
	}

	return nil
}
