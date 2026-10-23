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

package studio

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alternayte/auth-all/humaauth"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

const (
	// gatewayTokenPrefix marks a gateway token, so a leaked one is easy to
	// find in logs and secret scanners.
	gatewayTokenPrefix = "rgt_"
	// ArtifactVersionHeader carries the version number of a pulled artifact.
	ArtifactVersionHeader = "Restitch-Artifact-Version"
)

type GatewayToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix" doc:"The first characters of the token, to tell tokens apart."`
	CreatedAt  time.Time  `json:"created_at"`
	CreatedBy  string     `json:"created_by"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

func tokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newGatewayToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return gatewayTokenPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Server) registerGatewayTokens() {
	owner := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleOwner)}
	tags := []string{"gateway-tokens"}

	huma.Register(s.api, huma.Operation{
		OperationID: "issueGatewayToken", Method: http.MethodPost, Path: "/api/v1/environments/{env}/gateway-tokens",
		Summary:     "Issue a gateway token for one environment",
		Description: "The response holds the token. Studio stores only its digest, so this is the only time it is shown.",
		Tags:        tags, Middlewares: owner,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
		Body        struct {
			Name string `json:"name" minLength:"1" maxLength:"100"`
		}
	}) (*struct {
		Body struct {
			GatewayToken
			Token string `json:"token" doc:"The token. Studio keeps only its digest, so this is the only time it is shown."`
		}
	}, error) {
		p, err := principal(ctx)
		if err != nil {
			return nil, err
		}
		if err := s.requireEnvironment(ctx, in.Environment); err != nil {
			return nil, err
		}
		token, err := newGatewayToken()
		if err != nil {
			return nil, err
		}
		gt := GatewayToken{
			ID: uuid.NewString(), Name: in.Body.Name, Prefix: token[:len(gatewayTokenPrefix)+6],
			CreatedAt: time.Now().UTC(), CreatedBy: p.User.Email,
		}
		if _, err := s.db.db.ExecContext(ctx, s.db.rebind(`
			INSERT INTO studio_gateway_tokens (id, environment, name, digest, prefix, created_at, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?)`),
			gt.ID, in.Environment, gt.Name, tokenDigest(token), gt.Prefix, gt.CreatedAt, gt.CreatedBy); err != nil {
			return nil, err
		}
		out := &struct {
			Body struct {
				GatewayToken
				Token string `json:"token" doc:"The token. Studio keeps only its digest, so this is the only time it is shown."`
			}
		}{}
		out.Body.GatewayToken = gt
		out.Body.Token = token
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "listGatewayTokens", Method: http.MethodGet, Path: "/api/v1/environments/{env}/gateway-tokens",
		Summary: "List an environment's gateway tokens", Tags: tags, Middlewares: owner,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
	}) (*struct {
		Body struct {
			Tokens []GatewayToken `json:"tokens"`
		}
	}, error) {
		rows, err := s.db.db.QueryContext(ctx, s.db.rebind(`
			SELECT id, name, prefix, created_at, created_by, revoked_at, last_used_at
			FROM studio_gateway_tokens WHERE environment = ? ORDER BY created_at`), in.Environment)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := &struct {
			Body struct {
				Tokens []GatewayToken `json:"tokens"`
			}
		}{}
		out.Body.Tokens = []GatewayToken{}
		for rows.Next() {
			var gt GatewayToken
			var revoked, used sql.NullTime
			if err := rows.Scan(&gt.ID, &gt.Name, &gt.Prefix, &gt.CreatedAt, &gt.CreatedBy, &revoked, &used); err != nil {
				return nil, err
			}
			if revoked.Valid {
				gt.RevokedAt = &revoked.Time
			}
			if used.Valid {
				gt.LastUsedAt = &used.Time
			}
			out.Body.Tokens = append(out.Body.Tokens, gt)
		}
		return out, rows.Err()
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "revokeGatewayToken", Method: http.MethodDelete, Path: "/api/v1/environments/{env}/gateway-tokens/{id}",
		Summary: "Revoke a gateway token", DefaultStatus: http.StatusNoContent, Tags: tags, Middlewares: owner,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
		ID          string `path:"id"`
	}) (*struct{}, error) {
		res, err := s.db.db.ExecContext(ctx, s.db.rebind(`
			UPDATE studio_gateway_tokens SET revoked_at = ? WHERE environment = ? AND id = ? AND revoked_at IS NULL`),
			time.Now().UTC(), in.Environment, in.ID)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, huma.Error404NotFound("no active token with that id in this environment")
		}
		return nil, nil
	})
}

// gatewayEnvironment resolves the bearer gateway token of r to its
// environment. It returns "" for a missing, unknown or revoked token.
func (s *Server) gatewayEnvironment(ctx context.Context, r *http.Request) (string, error) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || !strings.HasPrefix(token, gatewayTokenPrefix) {
		return "", nil
	}
	digest := tokenDigest(token)
	var env string
	err := s.db.db.QueryRowContext(ctx, s.db.rebind(
		`SELECT environment FROM studio_gateway_tokens WHERE digest = ? AND revoked_at IS NULL`), digest).Scan(&env)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if _, err := s.db.db.ExecContext(ctx, s.db.rebind(
		`UPDATE studio_gateway_tokens SET last_used_at = ? WHERE digest = ?`), time.Now().UTC(), digest); err != nil {
		slog.WarnContext(ctx, "record gateway token use", "error", err)
	}
	return env, nil
}

// gatewayHandler serves the routes gateways call with a gateway token.
// They sit outside /api/v1: a gateway token is not a user credential.
func (s *Server) gatewayHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /gateway/v1/artifact", s.serveArtifact)
	mux.HandleFunc("GET /gateway/v1/artifact/versions/{number}", s.serveArtifact)
	mux.HandleFunc("POST /gateway/v1/heartbeat", s.serveHeartbeat)
	mux.HandleFunc("POST /gateway/v1/usage", s.serveUsage)
	mux.HandleFunc("POST /gateway/v1/requests", s.serveRequests)
	return mux
}

// requireGatewayToken answers 401 unless r carries a live gateway token,
// and returns the token's environment.
func (s *Server) requireGatewayToken(w http.ResponseWriter, r *http.Request) (string, bool) {
	env, err := s.gatewayEnvironment(r.Context(), r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return "", false
	}
	if env == "" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="restitch-gateway"`)
		http.Error(w, "a valid gateway token is required", http.StatusUnauthorized)
		return "", false
	}
	return env, true
}

// serveArtifact answers the current version, or the immutable version the
// path names, as YAML with an ETag.
func (s *Server) serveArtifact(w http.ResponseWriter, r *http.Request) {
	env, ok := s.requireGatewayToken(w, r)
	if !ok {
		return
	}
	var err error

	var v *artifactVersion
	immutable := r.PathValue("number") != ""
	if immutable {
		n, convErr := strconv.Atoi(r.PathValue("number"))
		if convErr != nil || n < 1 {
			http.Error(w, "version must be a positive integer", http.StatusBadRequest)
			return
		}
		v, err = s.version(r.Context(), env, n)
	} else {
		v, err = s.latestVersion(r.Context(), env)
		if err == nil && v == nil {
			http.Error(w, fmt.Sprintf("environment %q has no published version", env), http.StatusNotFound)
			return
		}
	}
	var se huma.StatusError
	if errors.As(err, &se) {
		http.Error(w, err.Error(), se.GetStatus())
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	etag := fmt.Sprintf(`"%s-%d"`, env, v.Number)
	w.Header().Set("ETag", etag)
	w.Header().Set(ArtifactVersionHeader, strconv.Itoa(v.Number))
	if immutable {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write([]byte(v.YAML))
}
