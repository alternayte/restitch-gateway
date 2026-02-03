// Package main provides the entry point for the restitch gateway.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/restitch/restitch-gateway/internal/server"
)

const version = "0.1.0"

func main() {
	// Parse command-line flags
	port := flag.Int("port", 8080, "HTTP server port")
	tlsPort := flag.Int("tls-port", 8443, "HTTPS server port")
	logFormat := flag.String("log-format", "json", "Log format: json or text")
	certFile := flag.String("cert", "", "Path to TLS certificate file")
	keyFile := flag.String("key", "", "Path to TLS private key file")

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

	// Register test route
	srv.Router().Handle(http.MethodGet, "/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pong"))
	})

	// Check if TLS is configured
	tlsEnabled := *certFile != "" && *keyFile != ""

	// Print startup message
	if tlsEnabled {
		fmt.Printf("restitch v%s listening on :%d (HTTP) and :%d (HTTPS)\n", version, *port, *tlsPort)
	} else {
		fmt.Printf("restitch v%s listening on :%d (HTTP only, no TLS certificate provided)\n", version, *port)
	}

	// Channel to capture server errors
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

	// Wait for an error (servers run until error or signal)
	err := <-errChan
	fmt.Fprintf(os.Stderr, "%v\n", err)
	os.Exit(1)
}
