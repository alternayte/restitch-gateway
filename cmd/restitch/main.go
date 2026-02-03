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

	flag.Parse()

	// Validate log format
	if *logFormat != "json" && *logFormat != "text" {
		fmt.Fprintf(os.Stderr, "invalid log-format: %s (must be json or text)\n", *logFormat)
		os.Exit(1)
	}

	// Print startup message
	fmt.Printf("restitch v%s listening on :%d (HTTP) and :%d (HTTPS)\n", version, *port, *tlsPort)

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

	// Start HTTP server
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
