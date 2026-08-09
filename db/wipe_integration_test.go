package db_test

import (
	"context"
	"os"
	"strconv"
	"testing"

	mysql "github.com/vormialabs/vormia-go-driver-mysql"
	postgres "github.com/vormialabs/vormia-go-driver-postgresql"
	"github.com/vormialabs/vormia-go/db"
)

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func TestWipePostgres(t *testing.T) {
	host := os.Getenv("PG_TEST_HOST")
	if host == "" {
		t.Skip("PG_TEST_HOST not set; skipping postgres wipe integration test")
	}

	ctx := context.Background()
	port, _ := strconv.Atoi(getenv("PG_TEST_PORT", "5432"))
	conn, err := postgres.Open(postgres.Config{
		Host:     host,
		Port:     port,
		User:     getenv("PG_TEST_USER", "postgres"),
		Password: getenv("PG_TEST_PASSWORD", "postgres"),
		Database: getenv("PG_TEST_DB", "postgres"),
		SSLMode:  getenv("PG_TEST_SSLMODE", "disable"),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS wipe_test_users (id SERIAL PRIMARY KEY, name TEXT)`,
		`CREATE TABLE IF NOT EXISTS wipe_test_posts (id SERIAL PRIMARY KEY, title TEXT)`,
	} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	if err := db.Wipe(ctx, conn, "postgres"); err != nil {
		t.Fatalf("wipe: %v", err)
	}

	var count int
	err = conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pg_tables WHERE schemaname = 'public'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count after wipe: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 public tables after wipe, got %d", count)
	}
}

func TestWipeMySQL(t *testing.T) {
	host := os.Getenv("MYSQL_TEST_HOST")
	if host == "" {
		t.Skip("MYSQL_TEST_HOST not set; skipping mysql wipe integration test")
	}

	ctx := context.Background()
	port, _ := strconv.Atoi(getenv("MYSQL_TEST_PORT", "3306"))
	conn, err := mysql.Open(mysql.Config{
		Host:     host,
		Port:     port,
		User:     getenv("MYSQL_TEST_USER", "root"),
		Password: getenv("MYSQL_TEST_PASSWORD", "root"),
		Database: getenv("MYSQL_TEST_DB", "testdb"),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS wipe_test_users (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(255))`,
		`CREATE TABLE IF NOT EXISTS wipe_test_posts (id INT AUTO_INCREMENT PRIMARY KEY, title VARCHAR(255))`,
	} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	if err := db.Wipe(ctx, conn, "mysql"); err != nil {
		t.Fatalf("wipe: %v", err)
	}

	var count int
	err = conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count after wipe: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 tables after wipe, got %d", count)
	}
}
