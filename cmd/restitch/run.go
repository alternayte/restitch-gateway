package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	iofs "io/fs"
	"log/slog"
	"net/http"
	"os"

	"github.com/restitch/restitch-gateway/internal/composition"
	"github.com/restitch/restitch-gateway/internal/observability"
	"github.com/restitch/restitch-gateway/internal/server"
)

func runCmd(args []string) int {
	flags := flag.NewFlagSet("run", flag.ExitOnError)
	port := flags.Int("port", 8080, "HTTP server port")
	tlsPort := flags.Int("tls-port", 8443, "HTTPS server port")
	logFormat := flags.String("log-format", "json", "Log format: json or text")
	logLevel := flags.String("log-level", "info", "Log level: debug, info, warn, error")
	certFile := flags.String("cert", "", "Path to TLS certificate file")
	keyFile := flags.String("key", "", "Path to TLS private key file")
	configFile := flags.String("config", "restitch.yaml", "path to composition config file")
	flags.Parse(args)

	if err := observability.Setup(*logFormat, *logLevel); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	srv := server.New(server.Config{
		Port:    *port,
		TLSPort: *tlsPort,
	})

	srv.Router().Use(observability.RequestIDMiddleware)
	srv.Router().Use(server.NewLoggingMiddleware())

	cfg, loadErr := composition.LoadConfigFile(*configFile)
	if errors.Is(loadErr, iofs.ErrNotExist) {
		slog.Warn("no composition config found, starting with health endpoints only",
			"config_file", *configFile)
	} else if loadErr != nil {
		slog.Error("failed to load config file", "file", *configFile, "error", loadErr)
		return 1
	} else {
		compiledCfg, err := composition.CompileConfig(context.Background(), cfg)
		if err != nil {
			slog.Error("failed to compile config", "error", err)
			return 1
		}

		slog.Info("loaded composition config",
			"file", *configFile,
			"upstreams", len(compiledCfg.Config.Upstreams),
			"compositions", len(compiledCfg.Config.Compositions))

		compositionHandler := composition.NewHandler(compiledCfg, nil)
		compositionHandler.RegisterRoutes(srv.Router())
	}

	srv.Router().Handle(http.MethodGet, "/health", server.HealthHandler(srv))
	srv.Router().Handle(http.MethodGet, "/ready", server.ReadyHandler(srv))

	srv.Router().Finalize()

	tlsEnabled := *certFile != "" && *keyFile != ""

	if tlsEnabled {
		slog.Info("server starting", "port", *port, "tls_port", *tlsPort, "version", version)
	} else {
		slog.Info("server starting", "port", *port, "version", version)
	}

	errChan := make(chan error, 2)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	if tlsEnabled {
		go func() {
			if err := srv.ListenAndServeTLS(*certFile, *keyFile); err != nil && err != http.ErrServerClosed {
				errChan <- fmt.Errorf("HTTPS server error: %w", err)
			}
		}()
	}

	select {
	case err := <-errChan:
		slog.Error("server error", "error", err)
		return 1
	case <-srv.WaitForShutdownSignal():
		ctx, cancel := context.WithTimeout(context.Background(), server.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("shutdown error", "error", err)
			return 1
		}
		slog.Info("shutdown complete")
	}

	return 0
}
