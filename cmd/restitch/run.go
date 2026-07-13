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
	"github.com/restitch/restitch-gateway/internal/gwconfig"
	"github.com/restitch/restitch-gateway/internal/inbound"
	"github.com/restitch/restitch-gateway/internal/observability"
	"github.com/restitch/restitch-gateway/internal/ratelimit"
	"github.com/restitch/restitch-gateway/internal/server"
	"github.com/restitch/restitch-gateway/internal/upstream"
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
	_ = flags.Parse(args)

	// Load and parse gateway config (server/admin blocks + compositions)
	expanded, raw, readErr := gwconfig.ReadAndExpand(*configFile)
	noConfigFile := errors.Is(readErr, iofs.ErrNotExist)
	if readErr != nil && !noConfigFile {
		fmt.Fprintf(os.Stderr, "failed to load config file: %v\n", readErr)
		return 1
	}

	// Parse server/admin config from YAML (or use defaults if no file)
	var gwcfg *gwconfig.File
	if noConfigFile {
		gwcfg = &gwconfig.File{}
	} else {
		var err error
		gwcfg, err = gwconfig.LoadBytes(expanded, raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config error: %v\n", err)
			return 1
		}
	}
	gwconfig.ApplyEnvOverrides(gwcfg)

	// Flag overrides win over env overrides and YAML
	if isFlagSet(flags, "port") {
		gwcfg.Server.Port = *port
	}
	if isFlagSet(flags, "tls-port") {
		gwcfg.Server.TLSPort = *tlsPort
	}
	if isFlagSet(flags, "log-format") {
		gwcfg.Server.LogFormat = *logFormat
	}
	if isFlagSet(flags, "log-level") {
		gwcfg.Server.LogLevel = *logLevel
	}
	if isFlagSet(flags, "cert") {
		gwcfg.Server.TLSCert = *certFile
	}
	if isFlagSet(flags, "key") {
		gwcfg.Server.TLSKey = *keyFile
	}

	if err := observability.Setup(gwcfg.Server.LogFormat, gwcfg.Server.LogLevel); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	tracingShutdown, err := observability.SetupTracing(context.Background(), "restitch-gateway", version)
	if err != nil {
		slog.Error("failed to initialize tracing", "error", err)
		return 1
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tracingShutdown(ctx); err != nil {
			slog.Error("tracing shutdown error", "error", err)
		}
	}()

	srv := server.New(server.Config{
		Port:    gwcfg.Server.Port,
		TLSPort: gwcfg.Server.TLSPort,
	})

	if noConfigFile {
		slog.Warn("no composition config found, starting with health endpoints only",
			"config_file", *configFile)

		router := server.NewRouter()
		router.Use(observability.RequestIDMiddleware)
		router.Use(server.NewLoggingMiddleware())
		router.Handle(http.MethodGet, "/health", server.HealthHandler(srv))
		router.Handle(http.MethodGet, "/ready", server.ReadyHandler(srv))
		router.Finalize()
		srv.SetHandler(router)
	} else {
		cfg, err := composition.ParseConfig(expanded)
		if err != nil {
			slog.Error("failed to parse config", "file", *configFile, "error", err)
			return 1
		}

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

		// Wire circuit breaker state gauge
		upstream.OnBreakerStateChange = func(name string, from, to string) {
			metrics.BreakerState.WithLabelValues(name).Set(observability.BreakerStateValue(to))
		}

		pipe := &Pipeline{
			Hash:     gwcfg.Hash(),
			Compiled: compiled,
			Executor: composition.NewExecutor(compiled),
		}

		var authenticator *inbound.Authenticator
		if gwcfg.Server.Auth != nil {
			authCfg := gwconfigToInboundAuth(gwcfg.Server.Auth)
			var authErr error
			authenticator, authErr = inbound.New(context.Background(), authCfg)
			if authErr != nil {
				slog.Error("failed to initialize inbound auth", "error", authErr)
				return 1
			}
			slog.Info("inbound authentication enabled",
				"api_keys", len(gwcfg.Server.Auth.APIKeys),
				"jwt", gwcfg.Server.Auth.JWT != nil)
		}

		handler := composition.NewHandler(compiled, authenticator)

		// Wire request recording (ring buffer + stats)
		ringSize := gwcfg.Admin.RequestLogSize
		ring := admin.NewRingBuffer(ringSize)
		stats := admin.NewStats()

		// Create time-series storage backend from config.
		adminCfg := gwcfg.Admin
		var store admin.Storage
		switch adminCfg.Storage.Type {
		case "sqlite":
			var err error
			retention := admin.ParseRangeDuration(adminCfg.Storage.Retention, 7*24*time.Hour)
			store, err = admin.NewSQLStorage(adminCfg.Storage.URL, adminCfg.Storage.AuthToken, retention)
			if err != nil {
				slog.Error("failed to open SQLite storage", "error", err)
				// fall back to memory
				store = admin.NewMemoryStorage(retention)
			}
		case "turso":
			var err error
			retention := admin.ParseRangeDuration(adminCfg.Storage.Retention, 30*24*time.Hour)
			store, err = admin.NewSQLStorage(adminCfg.Storage.URL, adminCfg.Storage.AuthToken, retention)
			if err != nil {
				slog.Error("failed to connect to Turso", "error", err)
				store = admin.NewMemoryStorage(retention)
			}
		default:
			retention := admin.ParseRangeDuration(adminCfg.Storage.Retention, 24*time.Hour)
			store = admin.NewMemoryStorage(retention)
		}

		acc := admin.NewAccumulator()

		// Start flush goroutine: periodically drains the accumulator into
		// durable storage and compacts old data.
		flushCtx, flushCancel := context.WithCancel(context.Background())
		go func() {
			ticker := time.NewTicker(60 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					buckets := acc.Flush()
					for _, b := range buckets {
						if err := store.RecordBucket(context.Background(), b); err != nil {
							slog.Error("failed to record time-series bucket", "error", err)
						}
					}
					retentionDur := admin.ParseRangeDuration(adminCfg.Storage.Retention, 24*time.Hour)
					if err := store.Compact(context.Background(), retentionDur); err != nil {
						slog.Error("failed to compact time-series storage", "error", err)
					}
				case <-flushCtx.Done():
					return
				}
			}
		}()
		defer func() {
			flushCancel()
			buckets := acc.Flush()
			for _, b := range buckets {
				if err := store.RecordBucket(context.Background(), b); err != nil {
					slog.Error("failed to flush bucket on shutdown", "error", err)
				}
			}
		}()

		handler.SetRecorder(&admin.MultiRecorder{Ring: ring, Stats: stats, Accumulator: acc, Storage: store})

		router := server.NewRouter()
		handler.RegisterRoutes(router)
		router.Handle(http.MethodGet, "/health", server.HealthHandler(srv))
		router.Handle(http.MethodGet, "/ready", server.ReadyHandler(srv))
		router.Finalize()
		pipe.Handler = router

		swapper := newSwapper(pipe)

		middlewares := []func(http.Handler) http.Handler{
			observability.RequestIDMiddleware,
			server.NewLoggingMiddleware(),
			recoveryMiddleware,
		}

		// Global gateway rate limiter
		if gwcfg.Server.RateLimit != nil {
			rl := gwcfg.Server.RateLimit
			globalLimiter := ratelimit.New(ratelimit.Config{
				RequestsPerSecond: rl.RequestsPerSecond,
				Burst:             rl.Burst,
				Key:               rl.Key,
			})
			middlewares = append(middlewares, globalLimiter.Middleware)
			slog.Info("global rate limit enabled",
				"rps", rl.RequestsPerSecond,
				"burst", rl.Burst,
				"key", rl.Key)
		}

		wrapped := applyMiddleware(swapper, middlewares...)
		srv.SetHandler(wrapped)

		// Reload function shared between admin API, SIGHUP, and fsnotify
		doReload := func() (string, error) {
			newExpanded, newRaw, err := gwconfig.ReadAndExpand(*configFile)
			if err != nil {
				return "", err
			}
			newGwcfg, err := gwconfig.LoadBytes(newExpanded, newRaw)
			if err != nil {
				return "", err
			}
			newHash := newGwcfg.Hash()

			if current := swapper.Current(); current != nil && current.Hash == newHash && newHash != "" {
				slog.Info("config unchanged", "hash", newHash)
				return newHash, nil
			}

			newCfg, err := composition.ParseConfig(newExpanded)
			if err != nil {
				return "", err
			}
			newCompiled, err := composition.CompileConfig(context.Background(), newCfg)
			if err != nil {
				return "", err
			}

			var newAuth *inbound.Authenticator
			if newGwcfg.Server.Auth != nil {
				authCfg := gwconfigToInboundAuth(newGwcfg.Server.Auth)
				newAuth, err = inbound.New(context.Background(), authCfg)
				if err != nil {
					return "", fmt.Errorf("inbound auth: %w", err)
				}
			}

			newPipe := &Pipeline{
				Hash:     newHash,
				Compiled: newCompiled,
				Executor: composition.NewExecutor(newCompiled),
			}
			newHandler := composition.NewHandler(newCompiled, newAuth)
			newRouter := server.NewRouter()
			newHandler.RegisterRoutes(newRouter)
			newRouter.Handle(http.MethodGet, "/health", server.HealthHandler(srv))
			newRouter.Handle(http.MethodGet, "/ready", server.ReadyHandler(srv))
			newRouter.Finalize()
			newPipe.Handler = newRouter

			old := swapper.Swap(newPipe)
			if old != nil {
				time.AfterFunc(30*time.Second, old.Close)
			}

			slog.Info("config reloaded", "old_hash", old.Hash, "new_hash", newHash)
			return newHash, nil
		}

		// Start reload watchers (SIGHUP + fsnotify)
		rl := newReloader(*configFile, doReload)
		rl.watchSignals()
		rl.watchFile()

		// Build an upstream health checker for the admin API
		healthChecker := upstream.NewChecker(compiled.Upstreams, 10*time.Second)

		// Admin server
		adminSrv := admin.New(admin.Config{
			Enabled: gwcfg.Admin.IsEnabled(),
			Port:    gwcfg.Admin.Port,
			APIKey:  gwcfg.Admin.APIKey,
			Storage: adminCfg.Storage,
		}, admin.Deps{
			Metrics:    metrics.Handler(),
			Version:    version,
			ConfigPath: *configFile,
			ConfigHash: func() string { return swapper.Current().Hash },
			Compositions: func() []admin.CompositionInfo {
				return compositionsFromPipeline(swapper.Current())
			},
			Upstreams: func(ctx context.Context) []admin.UpstreamInfo {
				return upstreamsFromPipeline(swapper.Current(), healthChecker, ctx)
			},
			Requests: ring,
			Stats:    stats,
			Storage:  store,
			Validate: func(yamlBytes []byte) []string {
				_, err := composition.ParseConfig(yamlBytes)
				if err != nil {
					return []string{err.Error()}
				}
				return nil
			},
			Reload: rl.reload,
		})
		if err := adminSrv.Start(); err != nil {
			slog.Error("admin server failed to start", "error", err)
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := adminSrv.Shutdown(ctx); err != nil {
				slog.Error("admin server shutdown error", "error", err)
			}
		}()
		defer func() {
			if err := store.Close(); err != nil {
				slog.Error("failed to close storage", "error", err)
			}
		}()
	}

	tlsEnabled := gwcfg.Server.TLSCert != "" && gwcfg.Server.TLSKey != ""

	if tlsEnabled {
		slog.Info("server starting", "port", gwcfg.Server.Port, "tls_port", gwcfg.Server.TLSPort, "version", version)
	} else {
		slog.Info("server starting", "port", gwcfg.Server.Port, "version", version)
	}

	errChan := make(chan error, 2)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	if tlsEnabled {
		go func() {
			if err := srv.ListenAndServeTLS(gwcfg.Server.TLSCert, gwcfg.Server.TLSKey); err != nil && err != http.ErrServerClosed {
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

func isFlagSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
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

func compositionsFromPipeline(p *Pipeline) []admin.CompositionInfo {
	if p == nil || p.Compiled == nil {
		return nil
	}
	var result []admin.CompositionInfo
	for name, comp := range p.Compiled.Config.Compositions {
		ci := admin.CompositionInfo{
			Name:   name,
			Path:   comp.Path,
			Method: comp.Method,
			Public: comp.Public,
		}
		for _, step := range comp.Steps {
			si := admin.StepInfo{
				Name:     step.Name,
				Upstream: step.Upstream,
				Method:   step.Method,
				Optional: step.Optional,
			}
			if step.Timeout != nil {
				si.TimeoutMS = step.Timeout.Milliseconds()
			}
			if step.DependsOn != nil {
				si.DependsOn = step.DependsOn
			} else {
				si.DependsOn = []string{}
			}

			// Compute inferred deps: all deps minus explicit deps
			if cc, ok := p.Compiled.Compositions[name]; ok {
				if cs, ok := cc.Steps[step.Name]; ok {
					explicit := make(map[string]bool, len(step.DependsOn))
					for _, d := range step.DependsOn {
						explicit[d] = true
					}
					var inferred []string
					for _, d := range cs.Deps {
						if !explicit[d] {
							inferred = append(inferred, d)
						}
					}
					if inferred == nil {
						inferred = []string{}
					}
					si.InferredDeps = inferred
				}
			}
			if si.InferredDeps == nil {
				si.InferredDeps = []string{}
			}

			ci.Steps = append(ci.Steps, si)
		}
		if cc, ok := p.Compiled.Compositions[name]; ok && cc.ExecutionPlan != nil {
			ci.Waves = cc.ExecutionPlan.Waves
		}
		result = append(result, ci)
	}
	return result
}

func upstreamsFromPipeline(p *Pipeline, checker *upstream.Checker, ctx context.Context) []admin.UpstreamInfo {
	if p == nil || p.Compiled == nil {
		return nil
	}
	var healthMap map[string]upstream.HealthStatus
	if checker != nil {
		healthMap = checker.Check(ctx)
	}
	var result []admin.UpstreamInfo
	for name, up := range p.Compiled.Config.Upstreams {
		ui := admin.UpstreamInfo{
			Name:      name,
			URL:       up.URL,
			TimeoutMS: up.Timeout.Milliseconds(),
		}
		if up.Auth != nil {
			ui.AuthType = up.Auth.Type()
		} else {
			ui.AuthType = "none"
		}
		if hs, ok := healthMap[name]; ok {
			ui.Health = admin.UpstreamHealthInfo{
				Status:    hs.Status,
				LatencyMS: hs.LatencyMS,
				CheckedAt: hs.CheckedAt.Format(time.RFC3339),
				Error:     hs.Error,
			}
		}
		result = append(result, ui)
	}
	return result
}

func gwconfigToInboundAuth(auth *gwconfig.InboundAuthConfig) inbound.InboundAuthConfig {
	cfg := inbound.InboundAuthConfig{
		APIKeys: auth.APIKeys,
	}
	if auth.JWT != nil {
		cfg.JWT = &inbound.JWTConfig{
			JWKSURL:  auth.JWT.JWKSURL,
			Issuer:   auth.JWT.Issuer,
			Audience: auth.JWT.Audience,
		}
	}
	return cfg
}
