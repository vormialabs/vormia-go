// Package seed runs SQL seeder files against any contract.Database.
// Unlike migrate, there is no up/down pairing and no tracking table —
// seeders are expected to be idempotent (INSERT ... ON CONFLICT / IGNORE).
package seed

import (
	"context"
	"fmt"
	"io/fs"
	"sort"

	"github.com/vormialabs/vormia-go/contract"
)

// Run executes every *.sql file in src, in filename order.
// Returns the list of files that ran successfully. On error, the files
// already executed are still returned alongside the error.
func Run(ctx context.Context, conn contract.Database, src fs.FS) ([]string, error) {
	files, err := fs.Glob(src, "*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	var ran []string
	for _, f := range files {
		sqlText, err := fs.ReadFile(src, f)
		if err != nil {
			return ran, err
		}
		if _, err := conn.ExecContext(ctx, string(sqlText)); err != nil {
			return ran, fmt.Errorf("seed %s: %w", f, err)
		}
		ran = append(ran, f)
	}
	return ran, nil
}
