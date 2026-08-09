package db_test

import (
	"context"
	"testing"

	sqlite "github.com/vormialabs/vormia-go-driver-sqlite"
	"github.com/vormialabs/vormia-go/db"
)

func TestWipeSQLite(t *testing.T) {
	ctx := context.Background()
	conn, err := sqlite.Open(sqlite.Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE posts (id INTEGER PRIMARY KEY, title TEXT)`,
		`INSERT INTO users (name) VALUES ('Ada')`,
	} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	if err := db.Wipe(ctx, conn, "sqlite"); err != nil {
		t.Fatalf("wipe: %v", err)
	}

	rows, err := conn.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("list after wipe: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		t.Fatalf("expected no user tables after wipe, found %q", name)
	}

	// Empty wipe is a no-op.
	if err := db.Wipe(ctx, conn, "sqlite"); err != nil {
		t.Fatalf("second wipe: %v", err)
	}
}

func TestWipeUnsupportedDriver(t *testing.T) {
	ctx := context.Background()
	conn, err := sqlite.Open(sqlite.Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	err = db.Wipe(ctx, conn, "oracle")
	if err == nil || err.Error() != `wipe: unsupported driver "oracle"` {
		t.Fatalf("expected unsupported driver error, got %v", err)
	}
}
