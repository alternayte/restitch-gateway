package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

//go:embed all:dist
var distFS embed.FS

func main() {
	port := flag.Int("port", 3080, "studio server port")
	gatewayAdminURL := flag.String("gateway-admin-url", "http://localhost:9090", "gateway admin API URL")
	adminKey := flag.String("admin-key", "", "admin API key (X-Admin-Key header)")
	flag.Parse()

	if v := os.Getenv("STUDIO_PORT"); v != "" {
		fmt.Sscanf(v, "%d", port)
	}
	if v := os.Getenv("STUDIO_GATEWAY_ADMIN_URL"); v != "" {
		*gatewayAdminURL = v
	}
	if v := os.Getenv("STUDIO_ADMIN_KEY"); v != "" {
		*adminKey = v
	}

	mux := buildMux(*gatewayAdminURL, *adminKey)

	addr := fmt.Sprintf(":%d", *port)
	slog.Info("restitch-studio starting",
		"port", *port,
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

func buildMux(gatewayAdminURL, adminKey string) *http.ServeMux {
	target, err := url.Parse(gatewayAdminURL)
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
		if adminKey != "" {
			req.Header.Set("X-Admin-Key", adminKey)
		}
	}

	mux := http.NewServeMux()

	mux.Handle("/api/", proxy)
	mux.Handle("/metrics", proxy)

	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		log.Fatal(err)
	}
	spaHandler := spaFileServer(http.FS(sub))
	mux.Handle("/", spaHandler)

	return mux
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
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()

		fileServer.ServeHTTP(w, r)
	})
}
