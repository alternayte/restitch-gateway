package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/alternayte/restitch-gateway/internal/registry"
	"github.com/alternayte/restitch-gateway/internal/session"
)

//go:embed all:dist
var distFS embed.FS

func main() {
	port := flag.Int("port", 3080, "studio server port")
	bind := flag.String("bind", "127.0.0.1", "bind address; use 0.0.0.0 only for deliberate remote access")
	gatewayAdminURL := flag.String("gateway-admin-url", "http://localhost:9090", "gateway admin API URL")
	adminKey := flag.String("admin-key", "", "admin API key (X-Admin-Key header)")
	registryKey := flag.String("registry-key", "", "registry API key required from clients (X-Admin-Key header)")
	dbPath := flag.String("db-path", "./studio.db", "SQLite database path")
	noMigrate := flag.Bool("no-migrate", false, "skip auto-migration on startup")
	flag.Parse()

	if v := os.Getenv("STUDIO_PORT"); v != "" {
		_, _ = fmt.Sscanf(v, "%d", port)
	}
	if v := os.Getenv("STUDIO_BIND"); v != "" {
		*bind = v
	}
	if v := os.Getenv("STUDIO_GATEWAY_ADMIN_URL"); v != "" {
		*gatewayAdminURL = v
	}
	if v := os.Getenv("STUDIO_ADMIN_KEY"); v != "" {
		*adminKey = v
	}
	if v := os.Getenv("STUDIO_REGISTRY_KEY"); v != "" {
		*registryKey = v
	}
	if v := os.Getenv("STUDIO_DB_PATH"); v != "" {
		*dbPath = v
	}
	if os.Getenv("STUDIO_NO_MIGRATE") == "true" {
		*noMigrate = true
	}

	db, err := openDB(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if !*noMigrate {
		if err := registry.RunMigrations(db); err != nil {
			log.Fatal(err)
		}
	}

	store := registry.NewStore(db)
	registryAPI := NewRegistryAPI(store)

	sessionStore := session.NewStore(db)
	prefsAPI := NewPreferencesAPI(sessionStore)

	mux := buildMux(muxDeps{
		gatewayAdminURL: *gatewayAdminURL,
		adminKey:        *adminKey,
		registryKey:     *registryKey,
		registryAPI:     registryAPI,
		prefsAPI:        prefsAPI,
		sessionStore:    sessionStore,
	})

	addr := net.JoinHostPort(*bind, fmt.Sprintf("%d", *port))
	if !isLoopback(*bind) {
		slog.Warn("studio bound to a non-loopback address",
			"bind", *bind,
			"hint", "the SPA and the gateway admin proxy are exposed to the network; use this only in a trusted network")
	}
	slog.Info("restitch-studio starting",
		"port", *port,
		"bind", *bind,
		"gateway_admin_url", *gatewayAdminURL)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// muxDeps carries everything buildMux needs. It is a struct rather than a
// positional list because three of the five values are optional in tests.
type muxDeps struct {
	gatewayAdminURL string
	adminKey        string
	registryKey     string
	registryAPI     *RegistryAPI
	prefsAPI        *PreferencesAPI
	sessionStore    *session.Store
}

func buildMux(d muxDeps) *http.ServeMux {
	target, err := url.Parse(d.gatewayAdminURL)
	if err != nil {
		log.Fatalf("invalid gateway admin URL: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		ResponseHeaderTimeout: 15 * time.Second,
	}

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		if strings.HasPrefix(req.URL.Path, "/api/") {
			req.URL.Path = "/admin" + req.URL.Path
		}
		if d.adminKey != "" {
			req.Header.Set("X-Admin-Key", d.adminKey)
		}
	}

	mux := http.NewServeMux()

	// V1 routes (Studio-native) — registered before proxy catch-all. The
	// registry API is machine-facing (the gateway poller and automation); it
	// requires X-Admin-Key. Browser preferences are separate and stay
	// cookie-bound.
	if d.registryAPI != nil {
		regAuth := requireRegistryKey(d.registryKey)
		mux.Handle("POST /api/v1/configs/validate", regAuth(http.HandlerFunc(d.registryAPI.handleValidateConfig)))
		mux.Handle("POST /api/v1/configs", regAuth(http.HandlerFunc(d.registryAPI.handleCreateConfig)))
		mux.Handle("GET /api/v1/configs", regAuth(http.HandlerFunc(d.registryAPI.handleListConfigs)))
		mux.Handle("GET /api/v1/configs/{id}", regAuth(http.HandlerFunc(d.registryAPI.handleGetConfig)))
		mux.Handle("PUT /api/v1/configs/{id}", regAuth(http.HandlerFunc(d.registryAPI.handleUpdateConfigContent)))
		mux.Handle("PATCH /api/v1/configs/{id}", regAuth(http.HandlerFunc(d.registryAPI.handleUpdateConfigMetadata)))
		mux.Handle("DELETE /api/v1/configs/{id}", regAuth(http.HandlerFunc(d.registryAPI.handleDeleteConfig)))
		mux.Handle("GET /api/v1/configs/{id}/versions", regAuth(http.HandlerFunc(d.registryAPI.handleListVersions)))
		mux.Handle("POST /api/v1/configs/{id}/versions/{version}/activate", regAuth(http.HandlerFunc(d.registryAPI.handleActivateVersion)))
		mux.Handle("GET /api/v1/registry/bundle", regAuth(http.HandlerFunc(d.registryAPI.handleGetBundle)))
	}

	// Preferences routes always mint, so curl and other cookie-less clients work.
	if d.prefsAPI != nil && d.sessionStore != nil {
		prefsMW := session.Middleware(d.sessionStore, session.AlwaysMint)
		mux.Handle("GET /api/v1/preferences", prefsMW(http.HandlerFunc(d.prefsAPI.handleGetPreferences)))
		mux.Handle("PUT /api/v1/preferences", prefsMW(http.HandlerFunc(d.prefsAPI.handlePutPreferences)))
	}

	// Proxy routes (gateway admin pass-through). Not session-wrapped — these
	// are forwarded to another process that has no use for a Studio session.
	mux.Handle("/api/", proxy)
	mux.Handle("/metrics", proxy)

	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		log.Fatal(err)
	}
	spaHandler := spaFileServer(http.FS(sub))
	// Document requests mint; static assets do not, which avoids creating
	// several sessions for one cold page load.
	if d.sessionStore != nil {
		spaHandler = session.Middleware(d.sessionStore, session.MintOnDocument)(spaHandler)
	}
	mux.Handle("/", spaHandler)

	return mux
}

// isLoopback reports whether bind names the loopback interface. "localhost"
// counts, because it resolves to a loopback address on every supported host.
func isLoopback(bind string) bool {
	switch bind {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// spaFileServer serves files from the embedded filesystem, falling back to
// index.html for client-side routes (standard SPA pattern).
func spaFileServer(fsys http.FileSystem) http.Handler {
	fileServer := http.FileServer(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}

		f, err := fsys.Open(path)
		if err != nil {
			// Never fall back for hashed build artifacts. Serving index.html
			// for a missing /assets/*.js returns HTML with a 200, so the
			// browser fails to parse a module and renders a blank page with no
			// 404 to diagnose from — which is exactly what a build that skipped
			// the frontend step produces.
			if strings.HasPrefix(path, "/assets/") {
				http.NotFound(w, r)
				return
			}
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()

		fileServer.ServeHTTP(w, r)
	})
}
