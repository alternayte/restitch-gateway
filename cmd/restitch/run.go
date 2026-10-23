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
	iofs "io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/alternayte/restitch-gateway/v2/internal/composition"
	"github.com/alternayte/restitch-gateway/v2/internal/gwconfig"
	"github.com/alternayte/restitch-gateway/v2/internal/hotreload"
	"github.com/alternayte/restitch-gateway/v2/internal/inbound"
	"github.com/alternayte/restitch-gateway/v2/internal/observability"
	"github.com/alternayte/restitch-gateway/v2/internal/ratelimit"
	"github.com/alternayte/restitch-gateway/v2/internal/server"
	"github.com/alternayte/restitch-gateway/v2/internal/upstream"
)

// gatewayOptions holds the process settings of `restitch gateway`. set
// names the flags given on the command line or through their env twin.
type gatewayOptions struct {
	port            int
	tlsPort         int
	logFormat       string
	logLevel        string
	tlsCert         string
	tlsKey          string
	readTimeout     time.Duration
	writeTimeout    time.Duration
	shutdownTimeout time.Duration
	configFile      string
	registryURL     string
	registryKey     string
	pollInterval    time.Duration
	opsPort         int
	set             map[string]bool
}

func parseGatewayFlags(args []string, lookup func(string) (string, bool)) (*gatewayOptions, error) {
	var o gatewayOptions
	flags := flag.NewFlagSet("gateway", flag.ContinueOnError)
	flags.IntVar(&o.port, "port", 8080, "HTTP server port")
	flags.IntVar(&o.tlsPort, "tls-port", 8443, "HTTPS server port")
	flags.StringVar(&o.logFormat, "log-format", "json", "Log format: json or text")
	flags.StringVar(&o.logLevel, "log-level", "info", "Log level: debug, info, warn, error")
	flags.StringVar(&o.tlsCert, "tls-cert", "", "Path to TLS certificate file")
	flags.StringVar(&o.tlsKey, "tls-key", "", "Path to TLS private key file")
	flags.DurationVar(&o.readTimeout, "read-timeout", 30*time.Second, "HTTP read timeout")
	flags.DurationVar(&o.writeTimeout, "write-timeout", 30*time.Second, "HTTP write timeout")
	flags.DurationVar(&o.shutdownTimeout, "shutdown-timeout", 30*time.Second, "Graceful shutdown deadline")
	flags.StringVar(&o.configFile, "config", "restitch.yaml", "Path to the artifact file (standalone mode)")
	flags.StringVar(&o.registryURL, "registry-url", "", "Registry URL for polling mode")
	flags.StringVar(&o.registryKey, "registry-key", "", "Key for registry auth")
	flags.DurationVar(&o.pollInterval, "poll-interval", 10*time.Second, "Registry poll interval")
	flags.IntVar(&o.opsPort, "ops-port", 0, "Port for /metrics (0 turns it off)")
	if err := flags.Parse(args); err != nil {
		return nil, err
	}
	if err := gwconfig.BindEnv(flags, lookup); err != nil {
		return nil, err
	}
	o.set = map[string]bool{}
	flags.Visit(func(f *flag.Flag) { o.set[f.Name] = true })
	return &o, nil
}

func gatewayCmd(args []string) int {
	opts, err := parseGatewayFlags(args, os.LookupEnv)
	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(os.Stderr, "gateway: %v\n", err)
		}
		return 2
	}
	configFile := &opts.configFile
	registryURL := &opts.registryURL
	registryKey := &opts.registryKey
	pollInterval := &opts.pollInterval

	registryMode := *registryURL != ""
	if registryMode && opts.set["config"] {
		fmt.Fprintln(os.Stderr, "gateway: -config (RESTITCH_CONFIG) and -registry-url (RESTITCH_REGISTRY_URL) are mutually exclusive")
		return 2
	}

	// configPath is a display value for logs and the admin API: the config
	// file path in file mode, or the registry URL in registry mode.
	configPath := *configFile
	if registryMode {
		configPath = *registryURL
	}

	// Load and parse gateway config (server/admin blocks + compositions).
	// In registry mode, the initial fetch is blocking: any failure here is fatal.
	var expanded, raw []byte
	var noConfigFile bool
	var regClient *hotreload.RegistryClient

	if registryMode {
		var clientOpts []hotreload.ClientOption
		if *registryKey != "" {
			clientOpts = append(clientOpts, hotreload.WithAdminKey(*registryKey))
		}
		regClient = hotreload.NewRegistryClient(*registryURL, clientOpts...)

		result, err := regClient.Fetch(context.Background(), "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to fetch initial config from registry: %v\n", err)
			return 1
		}
		raw = result.YAML
		expandedStr, err := gwconfig.ExpandEnvStrict(string(raw))
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to expand env vars in registry config: %v\n", err)
			return 1
		}
		expanded = []byte(expandedStr)
	} else {
		var readErr error
		expanded, raw, readErr = gwconfig.ReadAndExpand(*configFile)
		noConfigFile = errors.Is(readErr, iofs.ErrNotExist)
		if readErr != nil && !noConfigFile {
			fmt.Fprintf(os.Stderr, "failed to load config file: %v\n", readErr)
			return 1
		}
	}

	// Parse server/admin config from YAML (or use defaults if no file)
	var gwcfg *gwconfig.File
	if noConfigFile {
		// LoadBytes applies defaults and validates, so the no-config path
		// gets the same treatment as a file path (finding L7).
		var err error
		gwcfg, err = gwconfig.LoadBytes([]byte{}, []byte{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "config error: %v\n", err)
			return 1
		}
	} else {
		var err error
		gwcfg, err = gwconfig.LoadBytes(expanded, raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config error: %v\n", err)
			return 1
		}
	}

	// Process settings come from flags and their env twins only.
	gwcfg.Server = gwconfig.ServerConfig{
		Port:            opts.port,
		TLSPort:         opts.tlsPort,
		TLSCert:         opts.tlsCert,
		TLSKey:          opts.tlsKey,
		ReadTimeout:     gwconfig.Duration{Duration: opts.readTimeout},
		WriteTimeout:    gwconfig.Duration{Duration: opts.writeTimeout},
		ShutdownTimeout: gwconfig.Duration{Duration: opts.shutdownTimeout},
		LogFormat:       opts.logFormat,
		LogLevel:        opts.logLevel,
	}

	// Env and flag overrides land after the initial validation, so validate
	// again: an invalid RESTITCH_PORT must fail here, not at bind time
	// (finding L7).
	if err := gwcfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		return 1
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
		Port:         gwcfg.Server.Port,
		TLSPort:      gwcfg.Server.TLSPort,
		ReadTimeout:  gwcfg.Server.ReadTimeout.Duration,
		WriteTimeout: gwcfg.Server.WriteTimeout.Duration,
	})

	// shutdown replaces the per-component defers: deferred closes ran LIFO,
	// which closed the store before the final accumulator flush. The
	// configured server.shutdown_timeout governs the drain window (default
	// 30s; finding L6).
	shutdown := &gatewayShutdown{
		servers: []func(context.Context) error{srv.Shutdown},
		timeout: gwcfg.Server.ShutdownTimeout.Duration,
	}

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
			"source", configPath,
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
		if cfg.Auth != nil {
			var authErr error
			authenticator, authErr = inbound.New(context.Background(), *cfg.Auth)
			if authErr != nil {
				slog.Error("failed to initialize inbound auth", "error", authErr)
				return 1
			}
			slog.Info("inbound authentication enabled",
				"api_keys", len(cfg.Auth.APIKeys),
				"jwt", cfg.Auth.JWT != nil)
		}

		handler := composition.NewHandler(compiled, authenticator)

		router := server.NewRouter()
		handler.RegisterRoutes(router)
		router.Handle(http.MethodGet, "/health", server.HealthHandler(srv))
		router.Handle(http.MethodGet, "/ready", server.ReadyHandler(srv))
		router.Finalize()
		pipe.Handler = pipelineRateLimit(router, cfg.RateLimit)

		swapper := newSwapper(pipe)

		middlewares := []func(http.Handler) http.Handler{
			observability.RequestIDMiddleware,
			server.NewLoggingMiddleware(),
			recoveryMiddleware,
		}

		wrapped := applyMiddleware(swapper, middlewares...)
		srv.SetHandler(wrapped)

		// reloadFromBytes compiles raw YAML and hot-swaps the pipeline. It is
		// shared between file-mode reload (SIGHUP/fsnotify/admin API) and
		// registry-mode reload (poller/SIGHUP/admin API).
		reloadFromBytes := func(rawYAML []byte) (string, error) {
			expandedStr, err := gwconfig.ExpandEnvStrict(string(rawYAML))
			if err != nil {
				return "", fmt.Errorf("env expansion: %w", err)
			}
			newExpanded := []byte(expandedStr)

			newGwcfg, err := gwconfig.LoadBytes(newExpanded, rawYAML)
			if err != nil {
				return "", fmt.Errorf("load config: %w", err)
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
			if newCfg.Auth != nil {
				newAuth, err = inbound.New(context.Background(), *newCfg.Auth)
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
			newPipe.Handler = pipelineRateLimit(newRouter, newCfg.RateLimit)

			old := swapper.Swap(newPipe)
			if old != nil {
				time.AfterFunc(30*time.Second, old.Close)
			}

			slog.Info("config reloaded", "old_hash", old.Hash, "new_hash", newHash)
			return newHash, nil
		}

		// doReload is the file-mode reload entry point: read the file, then
		// share the compile/swap logic with registry mode via reloadFromBytes.
		doReload := func() (string, error) {
			raw, err := os.ReadFile(*configFile)
			if err != nil {
				return "", fmt.Errorf("read config: %w", err)
			}
			return reloadFromBytes(raw)
		}

		bgCtx, bgCancel := context.WithCancel(context.Background())
		defer bgCancel()
		var rl *reloader

		shutdown.stopBackground = func() {
			bgCancel()
			if rl != nil {
				rl.stop()
			}
		}

		if registryMode {
			// Register registry metrics
			metrics.RegisterRegistryMetrics()

			poller := hotreload.NewPoller(regClient, *pollInterval, reloadFromBytes, metrics)

			go func() {
				if err := poller.Run(bgCtx); err != nil && !errors.Is(err, context.Canceled) {
					slog.Error("registry poller stopped unexpectedly", "error", err)
				}
			}()
			go func() {
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGHUP)
				defer signal.Stop(sigCh)
				for {
					select {
					case <-bgCtx.Done():
						return
					case <-sigCh:
						slog.Info("SIGHUP received, triggering registry poll")
						poller.Trigger()
					}
				}
			}()

			slog.Info("registry polling mode enabled",
				"url", *registryURL, "poll_interval", pollInterval.String())
		} else {
			rl = newReloader(*configFile, doReload)
			rl.watchSignals()
			rl.watchFile()
		}

		// The optional ops port serves /metrics only. The probes stay on the
		// data port, because a container platform probes one port.
		if opts.opsPort != 0 {
			opsMux := http.NewServeMux()
			opsMux.Handle("GET /metrics", metrics.Handler())
			opsSrv := &http.Server{
				Addr:              net.JoinHostPort("", strconv.Itoa(opts.opsPort)),
				Handler:           opsMux,
				ReadHeaderTimeout: 10 * time.Second,
			}
			go func() {
				if err := opsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					slog.Error("ops server failed", "error", err)
				}
			}()
			slog.Info("ops server starting", "port", opts.opsPort)
			shutdown.servers = append(shutdown.servers, opsSrv.Shutdown)
		}
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
		if err := shutdown.run(); err != nil {
			slog.Error("shutdown error", "error", err)
		}
		return 1
	case <-srv.WaitForShutdownSignal():
		if err := shutdown.run(); err != nil {
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

// pipelineRateLimit wraps h with the global rate limiter described by cfg,
// or returns h unchanged when cfg is nil. It is applied to each pipeline as
// it is built, so a reload that changes server.rate_limit takes effect
// instead of being frozen at startup (finding L11).
func pipelineRateLimit(h http.Handler, cfg *ratelimit.Config) http.Handler {
	if cfg == nil {
		return h
	}
	rl := ratelimit.New(*cfg)
	slog.Info("global rate limit enabled",
		"rps", cfg.RequestsPerSecond,
		"burst", cfg.Burst,
		"key", cfg.Key)
	return rl.Middleware(h)
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
