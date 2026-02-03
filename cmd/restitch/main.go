// Package main provides the entry point for the restitch gateway.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/restitch/restitch-gateway/internal/client"
	"github.com/restitch/restitch-gateway/internal/composition"
	"github.com/restitch/restitch-gateway/internal/observability"
	"github.com/restitch/restitch-gateway/internal/server"
)

func main() {
	// Parse command-line flags
	port := flag.Int("port", 8080, "HTTP server port")
	tlsPort := flag.Int("tls-port", 8443, "HTTPS server port")
	logFormat := flag.String("log-format", "json", "Log format: json or text")
	certFile := flag.String("cert", "", "Path to TLS certificate file")
	keyFile := flag.String("key", "", "Path to TLS private key file")
	configFile := flag.String("config", "restitch.yaml", "path to composition config file")

	flag.Parse()

	// Validate log format
	if *logFormat != "json" && *logFormat != "text" {
		fmt.Fprintf(os.Stderr, "invalid log-format: %s (must be json or text)\n", *logFormat)
		os.Exit(1)
	}

	// Create server
	srv := server.New(server.Config{
		Port:      *port,
		TLSPort:   *tlsPort,
		LogFormat: *logFormat,
	})

	// Apply middleware chain:
	// 1. RequestIDMiddleware first - generates/extracts request ID, stores in context
	// 2. LoggingMiddleware second - reads request ID from context for logging
	srv.Router().Use(observability.RequestIDMiddleware)
	srv.Router().Use(server.NewLoggingMiddleware(server.LogFormat(*logFormat)))

	// Load composition config if available
	var compositionHandler *composition.Handler
	if _, err := os.Stat(*configFile); err == nil {
		// Config file exists - load and compile it
		cfg, err := composition.LoadConfigFile(*configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config file %s: %v\n", *configFile, err)
			os.Exit(1)
		}

		compiledCfg, err := composition.CompileConfig(context.Background(), cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to compile config: %v\n", err)
			os.Exit(1)
		}

		slog.Info("loaded composition config",
			"file", *configFile,
			"upstreams", len(compiledCfg.Config.Upstreams),
			"compositions", len(compiledCfg.Config.Compositions))

		// Create HTTP client for upstream calls (reuse Phase 1's optimized client)
		httpClient := client.New()

		// Create composition handler
		compositionHandler = composition.NewHandler(compiledCfg, httpClient.HTTPClient())

		// Register composition routes BEFORE health endpoints
		// This ensures composition routes take precedence
		compositionHandler.RegisterRoutes(srv.Router())
	} else {
		// Config file doesn't exist - start without compositions
		slog.Warn("no composition config found, starting with health endpoints only",
			"config_file", *configFile)
	}

	// Register health endpoints
	srv.Router().Handle(http.MethodGet, "/health", server.HealthHandler(srv))
	srv.Router().Handle(http.MethodGet, "/ready", server.ReadyHandler(srv))

	// Register test route
	srv.Router().Handle(http.MethodGet, "/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pong"))
	})

	// Check if TLS is configured
	tlsEnabled := *certFile != "" && *keyFile != ""

	// Print startup message
	if tlsEnabled {
		fmt.Printf("restitch v%s listening on :%d (HTTP) and :%d (HTTPS)\n", server.Version, *port, *tlsPort)
	} else {
		fmt.Printf("restitch v%s listening on :%d (HTTP only, no TLS certificate provided)\n", server.Version, *port)
	}

	// Channel to capture server startup errors
	errChan := make(chan error, 2)

	// Start HTTP server in goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	// Start HTTPS server if TLS is configured
	if tlsEnabled {
		go func() {
			if err := srv.ListenAndServeTLS(*certFile, *keyFile); err != nil && err != http.ErrServerClosed {
				errChan <- fmt.Errorf("HTTPS server error: %w", err)
			}
		}()
	}

	// Wait for either a server error or shutdown signal
	select {
	case err := <-errChan:
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	case <-srv.WaitForShutdownSignal():
		// Shutdown signal received, perform graceful shutdown
		if err := srv.Shutdown(srv.ShutdownContext()); err != nil {
			fmt.Fprintf(os.Stderr, "shutdown error: %v\n", err)
			os.Exit(1)
		}
	}
}
