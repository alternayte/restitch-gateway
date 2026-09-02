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
	"crypto/tls"
	"fmt"
	"os"
)

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
