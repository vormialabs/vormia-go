package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/vormialabs/vormia-go/contract"
)

// Wipe drops every user table on conn. Table enumeration is dialect-specific
// because the Database contract deliberately hides which engine is underneath.
//
// SQLite and PostgreSQL wrap the drop sequence in a transaction. MySQL does
// not — dropping many tables is non-atomic there; FOREIGN_KEY_CHECKS is toggled
// around the drops. Always treat Wipe as destructive.
func Wipe(ctx context.Context, conn contract.Database, driver string) error {
	switch strings.ToLower(driver) {
	case "sqlite":
		return wipeSQLite(ctx, conn)
	case "postgres", "postgresql":
		return wipePostgres(ctx, conn)
	case "mysql":
		return wipeMySQL(ctx, conn)
	default:
		return fmt.Errorf("wipe: unsupported driver %q", driver)
	}
}

func wipeSQLite(ctx context.Context, conn contract.Database) error {
	rows, err := conn.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return fmt.Errorf("wipe sqlite: list tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("wipe sqlite: scan: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("wipe sqlite: rows: %w", err)
	}
	if len(tables) == 0 {
		return nil
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("wipe sqlite: begin: %w", err)
	}
	for _, t := range tables {
		if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS "`+escapeIdent(t)+`"`); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("wipe sqlite: drop %s: %w", t, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("wipe sqlite: commit: %w", err)
	}
	return nil
}

func wipePostgres(ctx context.Context, conn contract.Database) error {
	rows, err := conn.QueryContext(ctx,
		`SELECT tablename FROM pg_tables WHERE schemaname = 'public'`)
	if err != nil {
		return fmt.Errorf("wipe postgres: list tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("wipe postgres: scan: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("wipe postgres: rows: %w", err)
	}
	if len(tables) == 0 {
		return nil
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("wipe postgres: begin: %w", err)
	}
	for _, t := range tables {
		if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS "`+escapeIdent(t)+`" CASCADE`); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("wipe postgres: drop %s: %w", t, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("wipe postgres: commit: %w", err)
	}
	return nil
}

// wipeMySQL drops all tables in the current database. Not transactional —
// a partial failure cannot roll back earlier drops. FOREIGN_KEY_CHECKS is
// disabled for the duration of the drops.
func wipeMySQL(ctx context.Context, conn contract.Database) error {
	rows, err := conn.QueryContext(ctx,
		`SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE()`)
	if err != nil {
		return fmt.Errorf("wipe mysql: list tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("wipe mysql: scan: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("wipe mysql: rows: %w", err)
	}
	if len(tables) == 0 {
		return nil
	}

	if _, err := conn.ExecContext(ctx, `SET FOREIGN_KEY_CHECKS=0`); err != nil {
		return fmt.Errorf("wipe mysql: disable fk checks: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, `SET FOREIGN_KEY_CHECKS=1`)
	}()

	for _, t := range tables {
		if _, err := conn.ExecContext(ctx, "DROP TABLE IF EXISTS `"+escapeBacktick(t)+"`"); err != nil {
			return fmt.Errorf("wipe mysql: drop %s: %w", t, err)
		}
	}
	return nil
}

func escapeIdent(name string) string {
	return strings.ReplaceAll(name, `"`, `""`)
}

func escapeBacktick(name string) string {
	return strings.ReplaceAll(name, "`", "``")
}
