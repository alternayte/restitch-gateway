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

// Package studio is the Restitch Studio control plane: sign-in, the Studio
// API and its OpenAPI document.
package studio

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	authall "github.com/alternayte/auth-all"
	"github.com/alternayte/auth-all/humaauth"
	"github.com/alternayte/auth-all/oauth/oidc"
	"github.com/alternayte/auth-all/plugins/admin"
	"github.com/alternayte/auth-all/plugins/apikeys"
	"github.com/alternayte/auth-all/plugins/roles"
	"github.com/alternayte/auth-all/ratelimit"
	"github.com/alternayte/auth-all/ratelimit/storelimit"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

// The Studio roles, lowest first.
const (
	RoleViewer = "viewer"
	RoleEditor = "editor"
	RoleOwner  = "owner"
)

// signUpPath is the auth-all public sign-up route. Studio creates users
// only through the bootstrap, the owner's admin routes and OIDC sign-in,
// so it routes nothing here. Remove this once auth-all can disable sign-up
// itself: https://github.com/alternayte/auth-all/issues/17
const signUpPath = "/sign-up/email"

// Config holds the Studio process settings.
type Config struct {
	DatabaseURL       string
	PublicURL         string
	BootstrapEmail    string
	BootstrapPassword string
	OIDCIssuer        string
	OIDCClientID      string
	OIDCClientSecret  string
	Logger            *slog.Logger
	Version           string
}

// Server is one Studio instance.
type Server struct {
	db      *database
	auth    *authall.Auth
	admin   *admin.Plugin
	apikeys *apikeys.Plugin
	now     func() time.Time
	api     huma.API
	handler http.Handler
}

// New opens and migrates the database, builds sign-in, creates the
// bootstrap owner when the database holds no user, and mounts the API.
func New(ctx context.Context, cfg Config) (*Server, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	public, err := url.Parse(cfg.PublicURL)
	if err != nil || public.Scheme == "" || public.Host == "" {
		return nil, fmt.Errorf("public url must be an absolute URL, got %q", cfg.PublicURL)
	}

	db, err := openDatabase(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	s := &Server{db: db, now: time.Now}
	if err := s.build(ctx, cfg, public); err != nil {
		_ = db.db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Server) build(ctx context.Context, cfg Config, public *url.URL) error {
	rolePlugin := roles.New(roles.Hierarchy(RoleViewer, RoleEditor, RoleOwner), roles.Default(RoleViewer))
	adminPlugin := admin.New(admin.AdminRole(RoleOwner))
	s.admin = adminPlugin
	s.apikeys = apikeys.New(apikeys.Prefix("rak_"))

	// A browser refuses a Secure cookie over plain HTTP, so a public URL
	// on http:// (restitch dev, a local quickstart) turns Secure off.
	secure := public.Scheme == "https"
	// The counters live in the database, so every Studio replica shares
	// one count.
	limiter, err := storelimit.New(s.db.auth, ratelimit.DefaultSignInRules())
	if err != nil {
		return fmt.Errorf("rate limiter: %w", err)
	}
	opts := []authall.Option{
		authall.WithStore(s.db.auth),
		authall.WithBaseURL(cfg.PublicURL),
		authall.WithEmailPassword(),
		authall.WithCookie(authall.CookieOptions{SameSite: http.SameSiteLaxMode, Secure: &secure}),
		authall.WithStrictOriginCheck(),
		authall.WithRateLimiter(limiter),
		authall.WithStrictRateLimiting(),
		authall.WithPlugins(rolePlugin, adminPlugin, s.apikeys),
		authall.WithLogger(cfg.Logger),
	}
	if cfg.OIDCIssuer != "" {
		opts = append(opts, authall.WithProvider(oidc.New(
			oidc.WithID("oidc"),
			oidc.WithIssuer(cfg.OIDCIssuer),
			oidc.WithClientID(cfg.OIDCClientID),
			oidc.WithClientSecret(cfg.OIDCClientSecret),
		)))
	}
	auth, err := authall.New(opts...)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	s.auth = auth

	if _, err := auth.Migrate(ctx); err != nil {
		return fmt.Errorf("auth migrations: %w", err)
	}
	if err := s.db.migrate(ctx); err != nil {
		return err
	}
	if cfg.BootstrapEmail != "" {
		created, err := adminPlugin.Bootstrap(ctx, admin.Credentials{
			Email:    cfg.BootstrapEmail,
			Password: cfg.BootstrapPassword,
		})
		if err != nil {
			return fmt.Errorf("bootstrap owner: %w", err)
		}
		if created {
			cfg.Logger.Info("created the bootstrap owner", "email", cfg.BootstrapEmail)
		}
	}

	apiMux := http.NewServeMux()
	humaCfg := huma.DefaultConfig("Restitch Studio", cfg.Version)
	humaCfg.OpenAPIPath = ""
	humaCfg.DocsPath = ""
	humaCfg.SchemasPath = ""
	humaCfg.CreateHooks = nil // no $schema links: the schemas route is off
	s.api = humago.New(apiMux, humaCfg)
	if err := humaauth.Register(s.api, auth); err != nil {
		return fmt.Errorf("merge auth api: %w", err)
	}
	dropOperation(s.api.OpenAPI(), auth.BasePath()+signUpPath, http.MethodPost)
	s.registerMe()
	s.registerDraft()
	s.registerPublish()
	s.registerHistory()
	s.registerGatewayTokens()
	s.registerInstances()
	s.registerUsage()
	s.registerRequests()
	s.registerImportExport()
	s.registerSchema()
	s.registerLint()
	s.registerSecrets()

	mux := http.NewServeMux()
	mux.Handle("POST "+auth.BasePath()+signUpPath, http.NotFoundHandler())
	mux.Handle(auth.BasePath()+"/", auth.Handler())
	mux.Handle("/api/v1/", auth.RequireAuth(apiMux))
	mux.Handle("/gateway/v1/", s.gatewayHandler())
	mux.Handle("/api/", http.NotFoundHandler())
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /ready", s.ready)
	mux.Handle("/", spaHandler())
	s.handler = mux
	return nil
}

// dropOperation removes one operation from the document, and the path when
// it has no operation left.
func dropOperation(doc *huma.OpenAPI, path, method string) {
	item, ok := doc.Paths[path]
	if !ok {
		return
	}
	if method == http.MethodPost {
		item.Post = nil
	}
	if item.Get == nil && item.Put == nil && item.Post == nil && item.Delete == nil &&
		item.Patch == nil && item.Head == nil && item.Options == nil && item.Trace == nil {
		delete(doc.Paths, path)
	}
}

// ready answers 200 while the database answers a ping, so a container
// platform holds traffic back from a replica that lost its database.
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.db.PingContext(ctx); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Handler serves the Studio routes.
func (s *Server) Handler() http.Handler { return s.handler }

// OpenAPIYAML returns the OpenAPI document of the Studio API, auth-all
// routes included.
func (s *Server) OpenAPIYAML() ([]byte, error) {
	return s.api.OpenAPI().YAML()
}

// Close closes the database.
func (s *Server) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.db.Close()
}

// principal returns the signed-in caller. RequireAuth guarantees one on
// every /api/v1 route.
func principal(ctx context.Context) (*authall.Principal, error) {
	p := authall.PrincipalFrom(ctx)
	if p == nil {
		return nil, errors.New("no principal on an authenticated route")
	}
	return p, nil
}
