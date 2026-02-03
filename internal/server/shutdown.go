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

// WaitForShutdown blocks until a shutdown signal is received, then performs
// graceful shutdown of the server. It:
// 1. Listens for SIGTERM and SIGINT signals
// 2. On signal received, sets ready state to false (so /ready returns 503)
// 3. Initiates graceful shutdown with 30-second timeout for connection draining
// 4. Returns nil on clean shutdown, error if shutdown times out
func (s *Server) WaitForShutdown() error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Block until signal received
	sig := <-quit
	fmt.Printf("shutdown signal received (%s), draining connections...\n", sig)

	// Immediately mark server as not ready
	// This causes /ready to return 503, telling load balancers to stop sending traffic
	s.SetReady(false)

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()

	// Perform graceful shutdown
	err := s.Shutdown(ctx)
	if err != nil {
		fmt.Printf("shutdown forced after timeout: %v\n", err)
		return err
	}

	fmt.Println("shutdown complete")
	return nil
}

// Shutdown gracefully shuts down both HTTP and HTTPS servers.
// It waits for in-flight requests to complete or until the context is cancelled.
// Returns an error if shutdown times out.
func (s *Server) Shutdown(ctx context.Context) error {
	var httpErr, httpsErr error

	// Shutdown HTTP server
	if s.httpServer != nil {
		httpErr = s.httpServer.Shutdown(ctx)
	}

	// Shutdown HTTPS server
	if s.httpsServer != nil {
		httpsErr = s.httpsServer.Shutdown(ctx)
	}

	// Return first error encountered
	if httpErr != nil {
		return fmt.Errorf("HTTP server shutdown: %w", httpErr)
	}
	if httpsErr != nil {
		return fmt.Errorf("HTTPS server shutdown: %w", httpsErr)
	}

	fmt.Println("shutdown complete")
	return nil
}

// WaitForShutdownSignal returns a channel that receives when a shutdown signal
// is received (SIGTERM or SIGINT). This allows the caller to handle shutdown
// in a select statement alongside other channels.
func (s *Server) WaitForShutdownSignal() <-chan struct{} {
	done := make(chan struct{})

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		sig := <-quit
		fmt.Printf("shutdown signal received (%s), draining connections...\n", sig)
		s.SetReady(false)
		close(done)
	}()

	return done
}

// ShutdownContext returns a context with the standard shutdown timeout (30 seconds).
// Use this context when calling Shutdown() to ensure proper connection draining.
func (s *Server) ShutdownContext() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), ShutdownTimeout)
	return ctx
}
