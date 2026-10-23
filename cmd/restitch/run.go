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
	studioURL       string
	gatewayToken    string
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
	flags.StringVar(&o.studioURL, "studio-url", "", "Studio URL to pull the environment's artifact from (managed mode)")
	flags.StringVar(&o.gatewayToken, "gateway-token", "", "Gateway token of the environment (managed mode)")
	flags.DurationVar(&o.pollInterval, "poll-interval", 10*time.Second, "How often to pull from Studio")
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
	managed := opts.studioURL != ""
	if managed && opts.set["config"] {
		fmt.Fprintln(os.Stderr, "gateway: -config (RESTITCH_CONFIG) and -studio-url (RESTITCH_STUDIO_URL) are mutually exclusive")
		return 2
	}
	if managed && opts.gatewayToken == "" {
		fmt.Fprintln(os.Stderr, "gateway: -studio-url needs -gateway-token (RESTITCH_GATEWAY_TOKEN)")
		return 2
	}

	// Process settings come from flags and their env twins only.
	gwcfg := &gwconfig.File{Server: gwconfig.ServerConfig{
		Port:            opts.port,
		TLSPort:         opts.tlsPort,
		TLSCert:         opts.tlsCert,
		TLSKey:          opts.tlsKey,
		ReadTimeout:     gwconfig.Duration{Duration: opts.readTimeout},
		WriteTimeout:    gwconfig.Duration{Duration: opts.writeTimeout},
		ShutdownTimeout: gwconfig.Duration{Duration: opts.shutdownTimeout},
		LogFormat:       opts.logFormat,
		LogLevel:        opts.logLevel,
	}}
	if err := gwcfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		return 2
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

	// shutdown runs the components down in a fixed order instead of defer
	// order (RESTITCH_SHUTDOWN_TIMEOUT bounds each stage).
	shutdown := &gatewayShutdown{
		servers: []func(context.Context) error{srv.Shutdown},
		timeout: gwcfg.Server.ShutdownTimeout.Duration,
	}

	metrics := observability.NewMetrics()
	observability.SetDefaultMetrics(metrics)
	upstream.OnBreakerStateChange = func(name string, from, to string) {
		metrics.BreakerState.WithLabelValues(name).Set(observability.BreakerStateValue(to))
	}

	// Every pipeline serves the probes. The first one serves only them:
	// standalone mode replaces it with the file, managed mode with the
	// first pulled version.
	withProbes := func(r *server.Router) {
		r.Handle(http.MethodGet, "/health", server.HealthHandler(srv))
		r.Handle(http.MethodGet, "/ready", server.ReadyHandler(srv))
		r.Finalize()
	}
	probeRouter := server.NewRouter()
	withProbes(probeRouter)
	swapper := newSwapper(&Pipeline{Handler: probeRouter})
	srv.SetHandler(applyMiddleware(swapper,
		observability.RequestIDMiddleware,
		server.NewLoggingMiddleware(),
		recoveryMiddleware,
	))

	// reloadFromBytes compiles one artifact and swaps it in. The file
	// watcher, SIGHUP and the Studio poller all apply versions through it.
	reloadFromBytes := func(rawYAML []byte) (string, error) {
		expandedStr, err := gwconfig.ExpandEnvStrict(string(rawYAML))
		if err != nil {
			return "", fmt.Errorf("env expansion: %w", err)
		}
		expanded := []byte(expandedStr)

		loaded, err := gwconfig.LoadBytes(expanded, rawYAML)
		if err != nil {
			return "", fmt.Errorf("load config: %w", err)
		}
		hash := loaded.Hash()
		if current := swapper.Current(); current != nil && current.Hash == hash {
			slog.Info("config unchanged", "hash", hash)
			return hash, nil
		}

		cfg, err := composition.ParseConfig(expanded)
		if err != nil {
			return "", err
		}
		compiled, err := composition.CompileConfig(context.Background(), cfg)
		if err != nil {
			return "", err
		}
		var authenticator *inbound.Authenticator
		if cfg.Auth != nil {
			authenticator, err = inbound.New(context.Background(), *cfg.Auth)
			if err != nil {
				return "", fmt.Errorf("inbound auth: %w", err)
			}
		}

		pipe := &Pipeline{
			Hash:     hash,
			Compiled: compiled,
			Executor: composition.NewExecutor(compiled),
		}
		router := server.NewRouter()
		composition.NewHandler(compiled, authenticator).RegisterRoutes(router)
		withProbes(router)
		pipe.Handler = pipelineRateLimit(router, cfg.RateLimit)

		old := swapper.Swap(pipe)
		time.AfterFunc(30*time.Second, old.Close)
		slog.Info("config applied",
			"hash", hash,
			"upstreams", len(compiled.Config.Upstreams),
			"compositions", len(compiled.Config.Compositions),
			"inbound_auth", cfg.Auth != nil)
		return hash, nil
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

	if managed {
		// A managed gateway starts without a config and stays not ready
		// until its first pull; a Studio outage never crash-loops it.
		srv.SetConfigured(false)
		metrics.RegisterRegistryMetrics()
		client := hotreload.NewClient(opts.studioURL, opts.gatewayToken)
		poller := hotreload.NewPoller(client, opts.pollInterval, reloadFromBytes, metrics)
		applied := make(chan struct{}, 1)
		poller.OnApplied(func(v int) {
			srv.SetConfigured(true)
			slog.Info("artifact version applied", "version", v)
			select {
			case applied <- struct{}{}:
			default:
			}
		})
		instanceID := hotreload.NewInstanceID()
		go hotreload.RunHeartbeat(bgCtx, client, hotreload.HeartbeatInterval, applied, func() hotreload.Heartbeat {
			st := poller.Status()
			return hotreload.Heartbeat{
				InstanceID: instanceID, BinaryVersion: version,
				AppliedVersion: st.Version, LastPullError: st.LastError,
			}
		})
		go func() {
			if err := poller.Run(bgCtx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("artifact poller stopped unexpectedly", "error", err)
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
					slog.Info("SIGHUP received, pulling the current version")
					poller.Trigger()
				}
			}
		}()
		slog.Info("managed mode", "studio_url", opts.studioURL, "poll_interval", opts.pollInterval.String())
	} else {
		raw, err := os.ReadFile(opts.configFile)
		switch {
		case errors.Is(err, iofs.ErrNotExist):
			slog.Warn("no artifact file, serving the probes only", "config_file", opts.configFile)
		case err != nil:
			slog.Error("failed to read the artifact file", "file", opts.configFile, "error", err)
			return 1
		default:
			if _, err := reloadFromBytes(raw); err != nil {
				slog.Error("failed to load the artifact file", "file", opts.configFile, "error", err)
				return 1
			}
			rl = newReloader(opts.configFile, func() (string, error) {
				raw, err := os.ReadFile(opts.configFile)
				if err != nil {
					return "", fmt.Errorf("read config: %w", err)
				}
				return reloadFromBytes(raw)
			})
			rl.watchSignals()
			rl.watchFile()
		}
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
