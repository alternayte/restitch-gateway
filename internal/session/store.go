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

package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Store provides data access for browser sessions and their preferences.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store wrapping db.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// EnsureSession inserts a row for id if none exists. It never overwrites
// existing preferences, so it is safe to call on every request.
func (s *Store) EnsureSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO browser_sessions (id) VALUES (?) ON CONFLICT(id) DO NOTHING`, id)
	if err != nil {
		return fmt.Errorf("ensure session: %w", err)
	}
	return nil
}

// GetPreferences returns the preferences for id along with whether they have
// ever been written. An unknown session, or one whose preferences column is
// still NULL, yields DefaultPreferences() and initialized=false.
func (s *Store) GetPreferences(ctx context.Context, id string) (Preferences, bool, error) {
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT preferences FROM browser_sessions WHERE id = ?`, id).Scan(&raw)
	switch {
	case err == sql.ErrNoRows:
		return DefaultPreferences(), false, nil
	case err != nil:
		return DefaultPreferences(), false, fmt.Errorf("get preferences: %w", err)
	}

	if !raw.Valid || raw.String == "" {
		return DefaultPreferences(), false, nil
	}

	prefs := DefaultPreferences()
	if err := json.Unmarshal([]byte(raw.String), &prefs); err != nil {
		return DefaultPreferences(), false, fmt.Errorf("decode preferences: %w", err)
	}
	if prefs.PinnedCompositions == nil {
		prefs.PinnedCompositions = []string{}
	}
	return prefs, true, nil
}

// PutPreferences persists p for id, creating the session row if needed.
func (s *Store) PutPreferences(ctx context.Context, id string, p Preferences) error {
	if p.PinnedCompositions == nil {
		p.PinnedCompositions = []string{}
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("encode preferences: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO browser_sessions (id, preferences) VALUES (?, ?)
		ON CONFLICT(id) DO UPDATE SET
			preferences = excluded.preferences,
			updated_at  = CURRENT_TIMESTAMP`, id, string(encoded))
	if err != nil {
		return fmt.Errorf("put preferences: %w", err)
	}
	return nil
}
