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
	"time"

	"github.com/google/uuid"
)

// PrepareDevEnvironment makes sure env exists and issues a fresh gateway
// token for it. restitch dev calls it on every start, because a token is
// shown only once and the in-process gateway needs one.
func (s *Server) PrepareDevEnvironment(ctx context.Context, env, by string) (string, error) {
	now := time.Now().UTC()
	if _, err := s.db.db.ExecContext(ctx, s.db.rebind(
		`INSERT INTO studio_environments (name, created_at, created_by) VALUES (?, ?, ?) ON CONFLICT (name) DO NOTHING`),
		env, now, by); err != nil {
		return "", err
	}
	// Earlier dev starts left a token behind. Nobody else ever held it, so
	// it goes away instead of piling up as revoked rows.
	if _, err := s.db.db.ExecContext(ctx, s.db.rebind(
		`DELETE FROM studio_gateway_tokens WHERE environment = ? AND name = ?`), env, "restitch dev"); err != nil {
		return "", err
	}
	token, err := newGatewayToken()
	if err != nil {
		return "", err
	}
	if _, err := s.db.db.ExecContext(ctx, s.db.rebind(`
		INSERT INTO studio_gateway_tokens (id, environment, name, digest, prefix, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)`),
		uuid.NewString(), env, "restitch dev", tokenDigest(token), token[:len(gatewayTokenPrefix)+6], now, by); err != nil {
		return "", err
	}
	return token, nil
}
