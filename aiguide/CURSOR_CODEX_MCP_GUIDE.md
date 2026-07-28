# vormia-go — AI Code Editor Guide

Instructions for AI coding assistants (Cursor, Codex, Copilot, MCP-based agents) working **with** or **on** the vormia-go package. Treat this file as authoritative context. Human-oriented explanations live in [GUIDE.md](GUIDE.md); architecture rationale in the [README](../README.md).

---

## 1. Package Facts (memorize these)

- **Module:** `github.com/vormialabs/vormia-go` — **v1.0.0**, requires **Go 1.26+**
- **Status:** Slice 1 — kernel + contracts only. No ORM, no validation, no auth, no CLI, no JSON helpers, no router-agnostic `Param`. Do not pretend these exist.
- **Two production packages, one file each:**
  - `contract` (`contract/contract.go`) — `Router`, `Database`, `Cache` interfaces. Stdlib imports only.
  - `app` (`app/kernel.go`) — `Kernel`, `New`, `WithDB`, `WithCache`, `Use`, `Routes`, `Run`.
- **Official drivers** (separate modules, chosen by the application, never by the framework):

| Import | Satisfies | Constructor |
|---|---|---|
| `github.com/vormialabs/vormia-go-driver-chi` | `contract.Router` | `chi.New()` |
| `github.com/vormialabs/vormia-go-driver-sqlite` | `contract.Database` | `sqlite.Open(sqlite.Config{Path: ...})` |
| `github.com/vormialabs/vormia-go-driver-postgresql` | `contract.Database` | `postgres.Open(postgres.Config{Host, User, Database, ...})` |
| `github.com/vormialabs/vormia-go-driver-mysql` | `contract.Database` | `mysql.Open(mysql.Config{...})` |
| `github.com/vormialabs/vormia-go-driver-redis` | `contract.Cache` | `redis.Open(redis.Config{Addr: ...})` |

## 2. Architectural Invariants (never violate)

1. **`contract` imports only the Go standard library.** Never add a third-party import to `contract/contract.go`.
2. **Framework production code (`app`, `contract`) never imports a concrete driver.** Drivers appear only in `_test.go` files and in end-user applications.
3. **Drivers never import vormia-go.** They satisfy contracts structurally. When writing a driver, do not add `vormia-go` to its `go.mod`.
4. **Contract changes are breaking changes** for every driver. Do not add, remove, or re-sign a contract method casually — flag it to the user first.
5. Router grouping (`Group`/`Route`) is deliberately **not** in `contract.Router`. Do not add it; it cannot be expressed portably.

## 3. Canonical Usage Patterns

### Bootstrap an application

```go
package main

import (
	"net/http"

	chi "github.com/vormialabs/vormia-go-driver-chi"
	postgres "github.com/vormialabs/vormia-go-driver-postgresql"
	redis "github.com/vormialabs/vormia-go-driver-redis"

	"github.com/vormialabs/vormia-go/app"
	"github.com/vormialabs/vormia-go/contract"
)

func main() {
	db, err := postgres.Open(postgres.Config{Host: "localhost", User: "app", Database: "app"})
	if err != nil { /* handle */ }
	cache, err := redis.Open(redis.Config{Addr: "localhost:6379"})
	if err != nil { /* handle */ }

	k := app.New(chi.New(), app.WithDB(db), app.WithCache(cache))

	// Middleware MUST be registered before Routes.
	k.Use(loggingMiddleware)

	k.Routes(func(r contract.Router) {
		r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		})
	})

	if err := k.Run(":8080"); err != nil { /* handle */ }
}
```

Ordering is fixed: **open drivers → `New` → `Use` → `Routes` → `Run`.** `Run` blocks until SIGINT/SIGTERM, then shuts down gracefully (10s budget) and closes DB/Cache. Do not add manual signal handling or `defer db.Close()` in `main` — `Run` owns the shutdown.

### Database access from a handler

Handlers reach drivers by closing over the kernel. Always pass the request context, and always wrap `?` placeholders in `Rebind` so the query is portable across sqlite/mysql (`?`) and postgres (`$1`):

```go
k.Routes(func(r contract.Router) {
	r.Get("/users", func(w http.ResponseWriter, req *http.Request) {
		rows, err := k.DB.QueryContext(req.Context(),
			k.DB.Rebind(`SELECT id, name FROM users WHERE active = ?`), true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		// scan rows...
	})
})
```

- Single row: `k.DB.QueryRowContext(ctx, q, args...).Scan(&dst)`
- Writes: `k.DB.ExecContext(ctx, q, args...)`
- Transactions: `tx, err := k.DB.BeginTx(ctx, nil)` then standard `*sql.Tx` usage (`tx.Commit()` / `tx.Rollback()`).
- `k.DB` may be **nil** if the app was built without `WithDB` — in framework code, nil-check; in app code that passed `WithDB`, don't bother.

### Cache access

`contract.Cache` is byte-oriented; serialize explicitly (JSON is the norm):

```go
// read: the bool means "found"; a miss is NOT an error
if b, ok, err := k.Cache.Get(ctx, "user:42"); err == nil && ok {
	_ = json.Unmarshal(b, &user)
}

// write
b, _ := json.Marshal(user)
_ = k.Cache.Set(ctx, "user:42", b, 5*time.Minute)
```

Never write `if err != nil` to detect a cache miss — check the `bool`.

### Middleware

Standard `net/http` shape, framework-agnostic:

```go
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
```

Register with `k.Use(loggingMiddleware)` **before** `k.Routes(...)` — chi panics if middleware is added after routes.

### URL parameters (Slice 1 caveat)

There is **no portable `Param` yet** (planned for Slice 2). If the app needs route parameters, use the concrete router's mechanism in the handler (e.g. `chi.URLParam(req, "id")` via `github.com/go-chi/chi/v5`) and note that this couples the handler to chi. Prefer query strings (`req.URL.Query().Get("id")`) when portability matters.

### JSON responses (Slice 1 caveat)

No helpers yet. Write them manually:

```go
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusOK)
_ = json.NewEncoder(w).Encode(payload)
```

## 4. Testing Patterns

### Test handlers in-process (no network, no Docker)

`contract.Router` embeds `ServeHTTP`, so drive it with `httptest`:

```go
k := app.New(chi.New(), app.WithDB(db))
k.Routes(func(r contract.Router) { /* register */ })

req := httptest.NewRequest(http.MethodGet, "/ping", nil)
rr := httptest.NewRecorder()
k.Router.ServeHTTP(rr, req)
// assert on rr.Code, rr.Body
```

Do **not** call `k.Run` in tests — it blocks on OS signals.

### Real database in tests

Use the sqlite driver with an in-memory DB — it is CGo-free (`modernc.org/sqlite`) and runs anywhere:

```go
db, err := sqlite.Open(sqlite.Config{Path: ":memory:"})
```

### Pin interface satisfaction at compile time

When adding or modifying a driver, keep/add the zero-cost assertion:

```go
var _ contract.Database = (*sqlite.DB)(nil)
```

### Run the suite

```bash
go test -v ./...
```

## 5. Writing a New Driver

A driver is a **separate Go module** that structurally satisfies exactly one contract interface.

1. New module, e.g. `github.com/vormialabs/vormia-go-driver-memory`. Do **not** require `vormia-go` in its `go.mod`.
2. For `Database` drivers: embed `*sql.DB` in your struct — every contract method except `Rebind` comes free. Implement `Rebind` (identity for `?`-style dialects; positional rewrite for postgres-style).
3. Export `Open(cfg Config) (*T, error)` (or `New()` for routers/in-memory caches) as the constructor. Ping/verify inside `Open` before returning.
4. Match contract signatures **exactly** — including `context.Context` first parameters and the `(value []byte, found bool, err error)` return of `Cache.Get`.
5. Add a compile-time assertion in the driver's own tests: `var _ contract.Cache = (*Client)(nil)` (importing `contract` in tests is acceptable; keeping it out of production driver code is preferred but the hard rule is the framework side).

## 6. Common Mistakes to Avoid

| Mistake | Correct behavior |
|---|---|
| Importing a driver inside `app/` or `contract/` production code | Drivers only in `_test.go` and user applications |
| Calling `k.Use(...)` after `k.Routes(...)` | Middleware first; chi panics otherwise |
| Treating a cache miss as an error | Check `Get`'s `bool`, not `err` |
| Writing `$1` placeholders directly | Write `?` and pass through `k.DB.Rebind(...)` |
| Passing `context.Background()` in handlers | Pass `req.Context()` so client disconnects cancel work |
| Calling `k.Run` in unit tests | Use `k.Router.ServeHTTP` with `httptest` |
| Adding `Group`, `Param`, JSON helpers, or ORM calls as if they exist | Slice 1 has none of these; note the gap or implement locally |
| Manual `defer db.Close()` alongside `k.Run` | `Run` closes attached drivers on shutdown |
| Adding fields to `Kernel` config via new `New` parameters | Add a functional `Option` (`WithX`) instead |

## 7. Repository Conventions (when editing vormia-go itself)

- Keep each package to its single file until size genuinely demands a split (`contract/contract.go`, `app/kernel.go`).
- Every exported identifier gets a doc comment; comments explain intent and trade-offs, not mechanics.
- Tests live in external test packages (`package app_test`) and exercise real drivers end-to-end rather than mocks.
- New optional kernel dependencies follow the existing pattern: field on `Kernel` (interface type, `// may be nil` comment), a `WithX` option, and closing in `closeDrivers` if it has a `Close`.
- Anything that would expand the contracts or add a framework dependency is an architectural decision — surface it to the user before implementing.
