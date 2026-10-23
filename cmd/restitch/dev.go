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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/alternayte/restitch-gateway/v2/internal/observability"
	"github.com/alternayte/restitch-gateway/v2/internal/studio"
)

// The dev sign-in. Studio binds loopback in dev mode, so these never face
// a network.
const (
	devEmail       = "dev@restitch.local"
	devPassword    = "restitch-dev-password"
	devEnvironment = "dev"
)

// devCmd runs Studio and a managed gateway in one process on SQLite, with
// no env vars: edit in the UI, publish, and the gateway serves it.
func devCmd(args []string) int {
	flags := flag.NewFlagSet("dev", flag.ContinueOnError)
	port := flags.Int("port", 8080, "gateway port")
	studioPort := flags.Int("studio-port", 3080, "Studio port (loopback only)")
	dataDir := flags.String("data-dir", ".restitch", "directory for the dev SQLite database")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if err := observability.Setup("text", "info"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "dev: %v\n", err)
		return 1
	}

	studioURL := "http://localhost:" + strconv.Itoa(*studioPort)
	ctx := context.Background()
	s, err := studio.New(ctx, studio.Config{
		DatabaseURL:       "sqlite:" + filepath.Join(*dataDir, "studio.db"),
		PublicURL:         studioURL,
		BootstrapEmail:    devEmail,
		BootstrapPassword: devPassword,
		Logger:            slog.Default(),
		Version:           version,
	})
	if err != nil {
		slog.Error("studio failed to start", "error", err)
		return 1
	}
	defer s.Close()
	token, err := s.PrepareDevEnvironment(ctx, devEnvironment, devEmail)
	if err != nil {
		slog.Error("dev environment", "error", err)
		return 1
	}

	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(*studioPort)))
	if err != nil {
		slog.Error("studio port", "error", err)
		return 1
	}
	studioSrv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := studioSrv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("studio server failed", "error", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = studioSrv.Shutdown(shutdownCtx)
	}()

	fmt.Fprintf(os.Stderr, "\n  Studio   %s  (sign in as %s / %s)\n  Gateway  http://localhost:%d  (environment %q; publish in Studio to serve)\n\n",
		studioURL, devEmail, devPassword, *port, devEnvironment)

	// The gateway runs in this process as a managed gateway of the dev
	// environment; it returns on SIGINT or SIGTERM.
	return gatewayCmd([]string{
		"-studio-url", studioURL,
		"-gateway-token", token,
		"-port", strconv.Itoa(*port),
		"-poll-interval", "1s",
		"-log-format", "text",
	})
}
