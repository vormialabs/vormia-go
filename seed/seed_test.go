package seed_test

import (
	"context"
	"testing"
	"testing/fstest"

	sqlite "github.com/vormialabs/vormia-go-driver-sqlite"
	"github.com/vormialabs/vormia-go/seed"
)

func TestRunOrder(t *testing.T) {
	ctx := context.Background()
	conn, err := sqlite.Open(sqlite.Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}

	src := fstest.MapFS{
		"02_b.sql": {Data: []byte(`INSERT INTO items (id, name) VALUES (2, 'b')`)},
		"01_a.sql": {Data: []byte(`INSERT INTO items (id, name) VALUES (1, 'a')`)},
	}

	ran, err := seed.Run(ctx, conn, src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(ran) != 2 || ran[0] != "01_a.sql" || ran[1] != "02_b.sql" {
		t.Fatalf("expected ordered files, got %v", ran)
	}

	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM items`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows, got %d", count)
	}
}

func TestRunEmpty(t *testing.T) {
	ctx := context.Background()
	conn, err := sqlite.Open(sqlite.Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	ran, err := seed.Run(ctx, conn, fstest.MapFS{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(ran) != 0 {
		t.Fatalf("expected no files, got %v", ran)
	}
}
