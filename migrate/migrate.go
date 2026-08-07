// Package migrate applies and rolls back SQL migrations against any
// contract.Database. It is engine-agnostic: the only per-engine concern,
// placeholder style, is handled by the driver's Rebind.
package migrate

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/vormialabs/vormia-go/contract"
)

const defaultTable = "schema_migrations"

// Migrator applies migrations from src to db.
type Migrator struct {
	db    contract.Database
	src   fs.FS
	table string
}

// New builds a Migrator. src holds <version>.up.sql / <version>.down.sql files.
func New(db contract.Database, src fs.FS) *Migrator {
	return &Migrator{db: db, src: src, table: defaultTable}
}

// ensure creates the tracking table if absent. The DDL is portable across
// SQLite, PostgreSQL, and MySQL.
func (m *Migrator) ensure(ctx context.Context) error {
	_, err := m.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+m.table+` (
		version VARCHAR(255) NOT NULL PRIMARY KEY,
		batch INTEGER NOT NULL,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

// versions lists all migration versions on disk, ascending.
func (m *Migrator) versions() ([]string, error) {
	ups, err := fs.Glob(m.src, "*.up.sql")
	if err != nil {
		return nil, err
	}
	vs := make([]string, 0, len(ups))
	for _, f := range ups {
		vs = append(vs, strings.TrimSuffix(f, ".up.sql"))
	}
	sort.Strings(vs) // timestamp-prefixed names sort into apply order
	return vs, nil
}

// applied returns version -> batch for everything already run.
func (m *Migrator) applied(ctx context.Context) (map[string]int, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT version, batch FROM `+m.table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var v string
		var b int
		if err := rows.Scan(&v, &b); err != nil {
			return nil, err
		}
		out[v] = b
	}
	return out, rows.Err()
}

// Up applies all pending migrations in one new batch, returning the versions run.
func (m *Migrator) Up(ctx context.Context) ([]string, error) {
	if err := m.ensure(ctx); err != nil {
		return nil, err
	}
	all, err := m.versions()
	if err != nil {
		return nil, err
	}
	done, err := m.applied(ctx)
	if err != nil {
		return nil, err
	}

	batch := maxBatch(done) + 1
	var run []string
	for _, v := range all {
		if _, ok := done[v]; ok {
			continue
		}
		sqlText, err := fs.ReadFile(m.src, v+".up.sql")
		if err != nil {
			return run, err
		}
		if err := m.applyOne(ctx, v, string(sqlText), batch); err != nil {
			return run, fmt.Errorf("migrate %s: %w", v, err)
		}
		run = append(run, v)
	}
	return run, nil
}

func (m *Migrator) applyOne(ctx context.Context, version, sqlText string, batch int) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, sqlText); err != nil {
		_ = tx.Rollback()
		return err
	}
	ins := m.db.Rebind(`INSERT INTO ` + m.table + ` (version, batch) VALUES (?, ?)`)
	if _, err := tx.ExecContext(ctx, ins, version, batch); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Rollback reverses migrations. steps <= 0 rolls back the most recent batch;
// steps > 0 rolls back that many, newest first.
func (m *Migrator) Rollback(ctx context.Context, steps int) ([]string, error) {
	done, err := m.applied(ctx)
	if err != nil {
		return nil, err
	}
	if len(done) == 0 {
		return nil, nil
	}

	versions := make([]string, 0, len(done))
	for v := range done {
		versions = append(versions, v)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(versions))) // newest first

	var target []string
	if steps <= 0 {
		last := maxBatch(done)
		for _, v := range versions {
			if done[v] == last {
				target = append(target, v)
			}
		}
	} else {
		for i, v := range versions {
			if i >= steps {
				break
			}
			target = append(target, v)
		}
	}

	var rolled []string
	for _, v := range target {
		down, err := fs.ReadFile(m.src, v+".down.sql")
		if err != nil {
			return rolled, fmt.Errorf("no down migration for %s: %w", v, err)
		}
		if err := m.rollbackOne(ctx, v, string(down)); err != nil {
			return rolled, fmt.Errorf("rollback %s: %w", v, err)
		}
		rolled = append(rolled, v)
	}
	return rolled, nil
}

func (m *Migrator) rollbackOne(ctx context.Context, version, downSQL string) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, downSQL); err != nil {
		_ = tx.Rollback()
		return err
	}
	del := m.db.Rebind(`DELETE FROM ` + m.table + ` WHERE version = ?`)
	if _, err := tx.ExecContext(ctx, del, version); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Reset rolls everything back, newest first.
func (m *Migrator) Reset(ctx context.Context) ([]string, error) {
	done, err := m.applied(ctx)
	if err != nil {
		return nil, err
	}
	return m.Rollback(ctx, len(done))
}

// Version returns the latest applied version, or "" if none.
func (m *Migrator) Version(ctx context.Context) (string, error) {
	if err := m.ensure(ctx); err != nil {
		return "", err
	}
	done, err := m.applied(ctx)
	if err != nil {
		return "", err
	}
	latest := ""
	for v := range done {
		if v > latest {
			latest = v
		}
	}
	return latest, nil
}

func maxBatch(applied map[string]int) int {
	max := 0
	for _, b := range applied {
		if b > max {
			max = b
		}
	}
	return max
}
