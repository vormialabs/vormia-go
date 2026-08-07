package db_test

import (
	"strings"
	"testing"

	"github.com/vormialabs/vormia-go-core/config"
	sqlite "github.com/vormialabs/vormia-go-driver-sqlite"
	"github.com/vormialabs/vormia-go/contract"
	"github.com/vormialabs/vormia-go/db"
)

func TestResolveDefaultAndNamed(t *testing.T) {
	cfg := config.New()
	cfg.Set("DB_DRIVER", "sqlite")
	cfg.Set("DB_PATH", ":memory:")
	cfg.Set("DB_MYSQL2_DRIVER", "mysql")
	cfg.Set("DB_MYSQL2_HOST", "127.0.0.1")
	cfg.Set("DB_MYSQL2_PORT", "3306")
	cfg.Set("DB_MYSQL2_USER", "app")
	cfg.Set("DB_MYSQL2_PASSWORD", "secret")
	cfg.Set("DB_MYSQL2_NAME", "appdb")

	reg := db.New(cfg)

	def, err := reg.Resolve("default")
	if err != nil {
		t.Fatalf("resolve default: %v", err)
	}
	if def.Driver != "sqlite" || def.Path != ":memory:" {
		t.Fatalf("default config: %+v", def)
	}

	named, err := reg.Resolve("mysql2")
	if err != nil {
		t.Fatalf("resolve mysql2: %v", err)
	}
	if named.Driver != "mysql" || named.Host != "127.0.0.1" || named.Port != "3306" ||
		named.User != "app" || named.Password != "secret" || named.Database != "appdb" {
		t.Fatalf("named config: %+v", named)
	}
	if named.Extra["HOST"] != "127.0.0.1" {
		t.Fatalf("expected Extra to hold group keys, got %+v", named.Extra)
	}
}

func TestConnectionOpenerDispatchAndCache(t *testing.T) {
	cfg := config.New()
	cfg.Set("DB_CONNECTION", "default")
	cfg.Set("DB_DRIVER", "sqlite")
	cfg.Set("DB_PATH", ":memory:")
	cfg.Set("DB_MYSQL2_DRIVER", "sqlite")
	cfg.Set("DB_MYSQL2_PATH", ":memory:")

	reg := db.New(cfg)

	var seen []db.ConnConfig
	reg.RegisterOpener("sqlite", func(c db.ConnConfig) (contract.Database, error) {
		seen = append(seen, c)
		return sqlite.Open(sqlite.Config{Path: c.Path})
	})

	conn1, err := reg.Connection("")
	if err != nil {
		t.Fatalf("default connection: %v", err)
	}
	if len(seen) != 1 || seen[0].Name != "default" || seen[0].Driver != "sqlite" {
		t.Fatalf("opener args: %+v", seen)
	}

	conn2, err := reg.Connection("")
	if err != nil {
		t.Fatalf("cached default: %v", err)
	}
	if conn1 != conn2 {
		t.Fatal("expected Connection to cache the live default connection")
	}
	if len(seen) != 1 {
		t.Fatalf("opener should not run again for cached connection, calls=%d", len(seen))
	}

	named, err := reg.Connection("mysql2")
	if err != nil {
		t.Fatalf("named connection: %v", err)
	}
	if named == nil {
		t.Fatal("expected named connection")
	}
	if len(seen) != 2 || seen[1].Name != "mysql2" {
		t.Fatalf("expected opener for mysql2, seen=%+v", seen)
	}

	if err := reg.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestConnectionErrors(t *testing.T) {
	cfg := config.New()
	cfg.Set("DB_DRIVER", "sqlite")
	cfg.Set("DB_PATH", ":memory:")
	reg := db.New(cfg)

	if _, err := reg.Resolve("missing"); err == nil || !strings.Contains(err.Error(), "DB_MISSING_*") {
		t.Fatalf("expected missing-group error, got %v", err)
	}

	if _, err := reg.Connection("default"); err == nil || !strings.Contains(err.Error(), "no opener registered") {
		t.Fatalf("expected missing-opener error, got %v", err)
	}

	cfg2 := config.New()
	cfg2.Set("DB_HOST", "localhost")
	reg2 := db.New(cfg2)
	reg2.RegisterOpener("sqlite", func(c db.ConnConfig) (contract.Database, error) {
		return sqlite.Open(sqlite.Config{Path: ":memory:"})
	})
	if _, err := reg2.Connection("default"); err == nil || !strings.Contains(err.Error(), "no DRIVER set") {
		t.Fatalf("expected missing-driver error, got %v", err)
	}
}
