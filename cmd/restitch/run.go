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
	"time"

	"github.com/restitch/restitch-gateway/internal/admin"
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

	cfg, loadErr := composition.LoadConfigFile(*configFile)
	if errors.Is(loadErr, iofs.ErrNotExist) {
		slog.Warn("no composition config found, starting with health endpoints only",
			"config_file", *configFile)

		router := server.NewRouter()
		router.Use(observability.RequestIDMiddleware)
		router.Use(server.NewLoggingMiddleware())
		router.Handle(http.MethodGet, "/health", server.HealthHandler(srv))
		router.Handle(http.MethodGet, "/ready", server.ReadyHandler(srv))
		router.Finalize()
		srv.SetHandler(router)
	} else if loadErr != nil {
		slog.Error("failed to load config file", "file", *configFile, "error", loadErr)
		return 1
	} else {
		compiled, err := composition.CompileConfig(context.Background(), cfg)
		if err != nil {
			slog.Error("failed to compile config", "error", err)
			return 1
		}

		slog.Info("loaded composition config",
			"file", *configFile,
			"upstreams", len(compiled.Config.Upstreams),
			"compositions", len(compiled.Config.Compositions))

		// Initialize Prometheus metrics
		metrics := observability.NewMetrics()
		observability.SetDefaultMetrics(metrics)

		pipe := &Pipeline{
			Compiled: compiled,
			Executor: composition.NewExecutor(compiled),
		}

		handler := composition.NewHandler(compiled, nil)

		// Wire request recording (ring buffer + stats)
		ring := admin.NewRingBuffer(500)
		stats := admin.NewStats()
		handler.SetRecorder(&admin.MultiRecorder{Ring: ring, Stats: stats})

		router := server.NewRouter()
		handler.RegisterRoutes(router)
		router.Handle(http.MethodGet, "/health", server.HealthHandler(srv))
		router.Handle(http.MethodGet, "/ready", server.ReadyHandler(srv))
		router.Finalize()
		pipe.Handler = router

		swapper := newSwapper(pipe)

		wrapped := applyMiddleware(swapper,
			observability.RequestIDMiddleware,
			server.NewLoggingMiddleware(),
			recoveryMiddleware,
		)
		srv.SetHandler(wrapped)

		// Reload function shared between admin API, SIGHUP, and fsnotify
		doReload := func() (string, error) {
			newCfg, err := composition.LoadConfigFile(*configFile)
			if err != nil {
				return "", err
			}
			newCompiled, err := composition.CompileConfig(context.Background(), newCfg)
			if err != nil {
				return "", err
			}

			newPipe := &Pipeline{
				Compiled: newCompiled,
				Executor: composition.NewExecutor(newCompiled),
			}
			newHandler := composition.NewHandler(newCompiled, nil)
			newRouter := server.NewRouter()
			newHandler.RegisterRoutes(newRouter)
			newRouter.Handle(http.MethodGet, "/health", server.HealthHandler(srv))
			newRouter.Handle(http.MethodGet, "/ready", server.ReadyHandler(srv))
			newRouter.Finalize()
			newPipe.Handler = newRouter

			hash := configHash(*configFile)
			newPipe.Hash = hash

			old := swapper.Swap(newPipe)
			if old != nil {
				time.AfterFunc(30*time.Second, old.Close)
			}

			return hash, nil
		}

		// Start reload watchers (SIGHUP + fsnotify)
		rl := newReloader(*configFile, doReload)
		rl.watchSignals()
		rl.watchFile()

		// Admin server
		adminSrv := admin.New(admin.Config{
			Enabled: true,
			Port:    9090,
		}, admin.Deps{
			Metrics: metrics.Handler(),
			Version:    version,
			ConfigPath: *configFile,
			ConfigHash: func() string { return swapper.Current().Hash },
			Requests:   ring,
			Stats:      stats,
			Validate: func(yamlBytes []byte) []string {
				_, err := composition.ParseConfig(yamlBytes)
				if err != nil {
					return []string{err.Error()}
				}
				return nil
			},
			Reload: rl.reload,
		})
		adminSrv.Start()
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			adminSrv.Shutdown(ctx)
		}()
	}

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

func applyMiddleware(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.ErrorContext(r.Context(), "panic recovered", "error", err)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
