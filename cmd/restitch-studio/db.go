package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// openDB opens the SQLite database at path and sets WAL journal mode.
// Migration is delegated to registry.RunMigrations by the caller.
func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	return db, nil
}
