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
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"testing"

	"github.com/alternayte/auth-all/store/postgres"
)

// isolatedPostgres creates a fresh schema in the test database and returns
// a URL whose search_path points at it, so tests never share tables.
func isolatedPostgres(t *testing.T, dsn string) string {
	t.Helper()
	db, err := postgres.Open(dsn)
	if err != nil {
		t.Fatalf("open %s: %v", dsn, err)
	}
	defer db.Close()

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "t_" + hex.EncodeToString(b)
	if _, err := db.Exec("CREATE SCHEMA " + name); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		if db, err := postgres.Open(dsn); err == nil {
			_, _ = db.Exec("DROP SCHEMA " + name + " CASCADE")
			_ = db.Close()
		}
	})

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse %s: %v", dsn, err)
	}
	q := u.Query()
	q.Set("search_path", name)
	u.RawQuery = q.Encode()
	return u.String()
}
