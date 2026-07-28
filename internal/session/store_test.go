package session

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (registers as "sqlite")

	"github.com/restitch/restitch-gateway/internal/registry"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := registry.RunMigrations(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewStore(db)
}

func TestStoreUninitializedUntilPut(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.EnsureSession(ctx, "sess-1"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	prefs, initialized, err := s.GetPreferences(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if initialized {
		t.Error("initialized = true before any PUT, want false")
	}
	if prefs.DefaultTimeRange != "1h" {
		t.Errorf("DefaultTimeRange = %q, want the 1h default", prefs.DefaultTimeRange)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.EnsureSession(ctx, "sess-1"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	want := Preferences{
		PinnedCompositions: []string{"comp-a", "comp-b"},
		SidebarCollapsed:   true,
		DefaultTimeRange:   "6h",
	}
	if err := s.PutPreferences(ctx, "sess-1", want); err != nil {
		t.Fatalf("PutPreferences: %v", err)
	}

	got, initialized, err := s.GetPreferences(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if !initialized {
		t.Error("initialized = false after PUT, want true")
	}
	if got.DefaultTimeRange != "6h" || !got.SidebarCollapsed {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if len(got.PinnedCompositions) != 2 || got.PinnedCompositions[0] != "comp-a" {
		t.Errorf("PinnedCompositions = %v, want %v", got.PinnedCompositions, want.PinnedCompositions)
	}
}

func TestStoreEnsureSessionIsIdempotent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.EnsureSession(ctx, "sess-1"); err != nil {
		t.Fatalf("first EnsureSession: %v", err)
	}
	if err := s.PutPreferences(ctx, "sess-1", Preferences{
		PinnedCompositions: []string{"keep-me"}, DefaultTimeRange: "1h",
	}); err != nil {
		t.Fatalf("PutPreferences: %v", err)
	}
	if err := s.EnsureSession(ctx, "sess-1"); err != nil {
		t.Fatalf("second EnsureSession: %v", err)
	}

	got, initialized, err := s.GetPreferences(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if !initialized || len(got.PinnedCompositions) != 1 {
		t.Errorf("EnsureSession clobbered existing preferences: %+v initialized=%v", got, initialized)
	}
}

func TestStoreUnknownSessionReturnsDefaults(t *testing.T) {
	s := testStore(t)
	prefs, initialized, err := s.GetPreferences(context.Background(), "never-seen")
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if initialized {
		t.Error("initialized = true for unknown session, want false")
	}
	if prefs.DefaultTimeRange != "1h" {
		t.Errorf("DefaultTimeRange = %q, want 1h", prefs.DefaultTimeRange)
	}
}

func TestStoreSessionsAreIsolated(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for _, id := range []string{"a", "b"} {
		if err := s.EnsureSession(ctx, id); err != nil {
			t.Fatalf("EnsureSession(%s): %v", id, err)
		}
	}
	if err := s.PutPreferences(ctx, "a", Preferences{
		PinnedCompositions: []string{"only-a"}, DefaultTimeRange: "24h",
	}); err != nil {
		t.Fatalf("PutPreferences: %v", err)
	}

	got, initialized, err := s.GetPreferences(ctx, "b")
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if initialized || len(got.PinnedCompositions) != 0 {
		t.Errorf("session b saw session a's preferences: %+v initialized=%v", got, initialized)
	}
}

func TestStorePutOverwrites(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.EnsureSession(ctx, "sess-1"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	first := Preferences{PinnedCompositions: []string{"x"}, DefaultTimeRange: "1h"}
	second := Preferences{PinnedCompositions: []string{"y", "z"}, DefaultTimeRange: "24h"}
	if err := s.PutPreferences(ctx, "sess-1", first); err != nil {
		t.Fatalf("first PutPreferences: %v", err)
	}
	if err := s.PutPreferences(ctx, "sess-1", second); err != nil {
		t.Fatalf("second PutPreferences: %v", err)
	}

	got, _, err := s.GetPreferences(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if got.DefaultTimeRange != "24h" || len(got.PinnedCompositions) != 2 {
		t.Errorf("got %+v, want %+v", got, second)
	}
}
