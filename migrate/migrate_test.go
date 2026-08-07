package migrate_test

import (
	"context"
	"testing"
	"testing/fstest"

	sqlite "github.com/vormialabs/vormia-go-driver-sqlite"
	"github.com/vormialabs/vormia-go/migrate"
)

func TestUpRollback(t *testing.T) {
	ctx := context.Background()

	db, err := sqlite.Open(sqlite.Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}

	src := fstest.MapFS{
		"20260101_create_users.up.sql":   {Data: []byte(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`)},
		"20260101_create_users.down.sql": {Data: []byte(`DROP TABLE users`)},
	}

	m := migrate.New(db, src)

	run, err := m.Up(ctx)
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	if len(run) != 1 {
		t.Fatalf("expected 1 migration, ran %d", len(run))
	}

	// Table exists now.
	if _, err := db.ExecContext(ctx, `INSERT INTO users (name) VALUES ('Ada')`); err != nil {
		t.Fatalf("table should exist after Up: %v", err)
	}

	// Running Up again is a no-op.
	if run2, _ := m.Up(ctx); len(run2) != 0 {
		t.Fatalf("second Up should be no-op, ran %d", len(run2))
	}

	ver, err := m.Version(ctx)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if ver != "20260101_create_users" {
		t.Fatalf("expected latest version, got %q", ver)
	}

	// Rollback removes the table.
	if _, err := m.Rollback(ctx, 0); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO users (name) VALUES ('x')`); err == nil {
		t.Fatal("table should be gone after rollback")
	}
}

func TestStatus(t *testing.T) {
	ctx := context.Background()

	db, err := sqlite.Open(sqlite.Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}

	src := fstest.MapFS{
		"20260101_a.up.sql":   {Data: []byte(`CREATE TABLE a (id INTEGER PRIMARY KEY)`)},
		"20260101_a.down.sql": {Data: []byte(`DROP TABLE a`)},
		"20260102_b.up.sql":   {Data: []byte(`CREATE TABLE b (id INTEGER PRIMARY KEY)`)},
		"20260102_b.down.sql": {Data: []byte(`DROP TABLE b`)},
	}

	m := migrate.New(db, src)

	rows, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Applied || rows[1].Applied {
		t.Fatal("nothing should be applied yet")
	}

	if _, err := m.Up(ctx); err != nil {
		t.Fatalf("up: %v", err)
	}

	rows, err = m.Status(ctx)
	if err != nil {
		t.Fatalf("status after up: %v", err)
	}
	for _, r := range rows {
		if !r.Applied || r.Batch != 1 {
			t.Fatalf("expected applied batch 1, got %+v", r)
		}
	}
}
