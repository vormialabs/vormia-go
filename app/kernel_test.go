package app_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	chi "github.com/vormialabs/vormia-go-driver-chi"
	sqlite "github.com/vormialabs/vormia-go-driver-sqlite"

	"github.com/vormialabs/vormia-go/app"
	"github.com/vormialabs/vormia-go/contract"
)

// The definitive "it all fits" check: real drivers satisfy the canonical
// contracts. If this block compiles, the architecture holds.
var (
	_ contract.Router   = (*chi.Router)(nil)
	_ contract.Database = (*sqlite.DB)(nil)
)

func TestKernelServesAndReadsDB(t *testing.T) {
	ctx := context.Background()

	db, err := sqlite.Open(sqlite.Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE t (msg TEXT NOT NULL)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO t (msg) VALUES ('hi')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	k := app.New(chi.New(), app.WithDB(db))

	k.Routes(func(r contract.Router) {
		r.Get("/ping", func(w http.ResponseWriter, req *http.Request) {
			var msg string
			// k.DB is the contract.Database — QueryRowContext comes from it.
			if err := k.DB.QueryRowContext(req.Context(),
				`SELECT msg FROM t`).Scan(&msg); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(msg))
		})
	})

	// Drive the router as a plain http.Handler — no real network needed.
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rr := httptest.NewRecorder()
	k.Router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	body, _ := io.ReadAll(rr.Body)
	if string(body) != "hi" {
		t.Fatalf("body: got %q want hi", string(body))
	}
}
