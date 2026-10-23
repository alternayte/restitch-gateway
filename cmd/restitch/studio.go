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
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/alternayte/restitch-gateway/v2/internal/gwconfig"
	"github.com/alternayte/restitch-gateway/v2/internal/observability"
	"github.com/alternayte/restitch-gateway/v2/internal/studio"
)

// studioOptions holds the process settings of `restitch studio`.
type studioOptions struct {
	port            int
	bind            string
	logFormat       string
	logLevel        string
	shutdownTimeout time.Duration
	cfg             studio.Config
}

func parseStudioFlags(args []string, lookup func(string) (string, bool)) (*studioOptions, error) {
	var o studioOptions
	flags := flag.NewFlagSet("studio", flag.ContinueOnError)
	flags.IntVar(&o.port, "port", 3080, "HTTP port")
	flags.StringVar(&o.bind, "bind", "", "Bind address (empty means every interface)")
	flags.StringVar(&o.logFormat, "log-format", "json", "Log format: json or text")
	flags.StringVar(&o.logLevel, "log-level", "info", "Log level: debug, info, warn, error")
	flags.DurationVar(&o.shutdownTimeout, "shutdown-timeout", 30*time.Second, "Graceful shutdown deadline")
	flags.StringVar(&o.cfg.DatabaseURL, "database-url", "", "postgres:// URL, or sqlite: plus a local file path (required)")
	flags.StringVar(&o.cfg.PublicURL, "public-url", "", "URL people use to reach Studio (default http://localhost:<port>)")
	flags.StringVar(&o.cfg.BootstrapEmail, "bootstrap-email", "", "Email of the first owner, created when no user exists")
	flags.StringVar(&o.cfg.BootstrapPassword, "bootstrap-password", "", "Password of the first owner")
	flags.StringVar(&o.cfg.OIDCIssuer, "oidc-issuer", "", "OIDC issuer URL for single sign-on (optional)")
	flags.StringVar(&o.cfg.OIDCClientID, "oidc-client-id", "", "OIDC client ID")
	flags.StringVar(&o.cfg.OIDCClientSecret, "oidc-client-secret", "", "OIDC client secret")
	if err := flags.Parse(args); err != nil {
		return nil, err
	}
	if err := gwconfig.BindEnv(flags, lookup); err != nil {
		return nil, err
	}
	if o.cfg.DatabaseURL == "" {
		return nil, errors.New("-database-url (RESTITCH_DATABASE_URL) is required")
	}
	if (o.cfg.BootstrapEmail == "") != (o.cfg.BootstrapPassword == "") {
		return nil, errors.New("-bootstrap-email and -bootstrap-password go together")
	}
	if o.cfg.OIDCIssuer != "" && (o.cfg.OIDCClientID == "" || o.cfg.OIDCClientSecret == "") {
		return nil, errors.New("-oidc-issuer needs -oidc-client-id and -oidc-client-secret")
	}
	if o.cfg.PublicURL == "" {
		o.cfg.PublicURL = "http://localhost:" + strconv.Itoa(o.port)
	}
	return &o, nil
}

func studioCmd(args []string) int {
	opts, err := parseStudioFlags(args, os.LookupEnv)
	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(os.Stderr, "studio: %v\n", err)
		}
		return 2
	}
	if err := observability.Setup(opts.logFormat, opts.logLevel); err != nil {
		fmt.Fprintf(os.Stderr, "studio: %v\n", err)
		return 2
	}
	opts.cfg.Logger = slog.Default()
	opts.cfg.Version = version

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s, err := studio.New(ctx, opts.cfg)
	if err != nil {
		slog.Error("studio failed to start", "error", err)
		return 1
	}
	defer s.Close()

	srv := &http.Server{
		Addr:              net.JoinHostPort(opts.bind, strconv.Itoa(opts.port)),
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	slog.Info("studio starting", "addr", srv.Addr, "public_url", opts.cfg.PublicURL, "version", version)

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("studio server failed", "error", err)
			return 1
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), opts.shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("studio shutdown", "error", err)
			return 1
		}
	}
	return 0
}
