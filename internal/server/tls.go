package server

import (
	"crypto/tls"
	"fmt"
	"os"
)

// TLSConfig holds TLS certificate configuration.
type TLSConfig struct {
	CertFile string // Path to TLS certificate file
	KeyFile  string // Path to TLS private key file
}

// LoadTLSConfig loads and validates TLS configuration from certificate files.
// Returns a configured tls.Config with modern security settings.
func LoadTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	// Verify certificate file exists
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("certificate file not found: %s", certFile)
	}

	// Verify key file exists
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("key file not found: %s", keyFile)
	}

	// Load certificate and key
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %w", err)
	}

	// Configure TLS with modern settings
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{
			tls.CurveP256,
			tls.X25519,
		},
	}

	return tlsConfig, nil
}
