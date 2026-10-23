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
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"github.com/alternayte/auth-all/schema"
	"github.com/alternayte/auth-all/store"
	"github.com/alternayte/auth-all/store/postgres"
	"github.com/alternayte/auth-all/store/sqlite"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// database is the Studio database: one handle shared by the auth-all store
// and the Studio tables.
type database struct {
	db      *sql.DB
	dialect schema.Dialect
	auth    store.Store
}

// openDatabase opens RESTITCH_DATABASE_URL: a postgres:// or postgresql://
// URL, or sqlite: followed by a file path. SQLite must sit on a local disk;
// WAL mode does not work on a network file share.
func openDatabase(url string) (*database, error) {
	switch {
	case strings.HasPrefix(url, "postgres://"), strings.HasPrefix(url, "postgresql://"):
		db, err := postgres.Open(url)
		if err != nil {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
		return &database{db: db, dialect: schema.Postgres, auth: postgres.New(db)}, nil
	case strings.HasPrefix(url, "sqlite:"):
		path := strings.TrimPrefix(url, "sqlite:")
		if path == "" {
			return nil, fmt.Errorf("database url %q names no file", url)
		}
		db, err := sqlite.Open("file:" + path)
		if err != nil {
			return nil, fmt.Errorf("open sqlite: %w", err)
		}
		return &database{db: db, dialect: schema.SQLite, auth: sqlite.New(db)}, nil
	default:
		return nil, fmt.Errorf("database url must start with postgres://, postgresql:// or sqlite:, got %q", url)
	}
}

// migrate applies the Studio migrations with goose. auth-all applies its
// own units through Auth.Migrate, so the two sets never share a version
// table.
func (d *database) migrate(ctx context.Context) error {
	dialect := goose.DialectSQLite3
	if d.dialect == schema.Postgres {
		dialect = goose.DialectPostgres
	}
	sub, err := fs.Sub(migrationFS, "migrations")
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(dialect, d.db, sub)
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("studio migrations: %w", err)
	}
	return nil
}

// rebind turns ? placeholders into $n for Postgres.
func (d *database) rebind(q string) string {
	if d.dialect != schema.Postgres {
		return q
	}
	var b strings.Builder
	n := 0
	for _, r := range q {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
