# vormia-go — AI Code Editor Guide

Instructions for AI coding assistants (Cursor, Codex, Copilot, MCP-based agents) working **with** or **on** the vormia-go package. Treat this file as authoritative context. Human-oriented explanations live in [GUIDE.md](GUIDE.md); architecture rationale in the [README](../README.md); releases in [RELEASE_NOTES.md](../RELEASE_NOTES.md).

---

## 1. Package Facts (memorize these)

- **Module:** `github.com/vormialabs/vormia-go` — **v1.2.0**, requires **Go 1.26+**
- **Status:** Kernel + contracts (incl. `Router.Routes()`) + `db` (registry + `Wipe`) + `migrate` + `seed` + `cache` registry. No ORM, no validation, no auth, no CLI commands, no `Cache.Flush`, no JSON helpers, no router-agnostic `Param`. Do not pretend those exist. Pair chi driver **v1.1.0+** for `Routes()`.
- **Production packages:**
  - `contract` (`contract/contract.go`) — `Router` (incl. `Routes()` / `RouteInfo`), `Database`, `Cache`. Stdlib imports only.
  - `app` (`app/kernel.go`) — `Kernel`, `New`, `WithDB`, `WithCache`, `Use`, `Routes`, `Run`.
  - `db` (`db/registry.go`, `db/wipe.go`) — `ConnConfig`, `Opener`, `Registry`, `Wipe`. Imports `contract` + `vormia-go-core/config` only.
  - `migrate` (`migrate/migrate.go`) — `Migrator` (`New`, `Up`, `Rollback`, `Reset`, `Version`, `Status`), `StatusRow`. Imports `contract` + stdlib only.
  - `seed` (`seed/seed.go`) — `Run(ctx, conn, src)`. Imports `contract` + stdlib only.
  - `cache` (`cache/registry.go`) — `ConnConfig`, `Opener`, `Registry` (`New`, `RegisterOpener`, `Default`, `Resolve`, `Connection`, `Close`). Imports `contract` + `vormia-go-core/config` only.
- **Core dependency:** `github.com/vormialabs/vormia-go-core@v1.1.0` for `config` (`GetString`, `Prefixed`). Core stays DB-agnostic; **vormia-go owns** the `DB_*` / `DB_<NAME>_*` and `CACHE_*` / `CACHE_<NAME>_*` naming rules.
- **Official drivers** (separate modules, chosen by the application, never by the framework):

| Import | Satisfies | Constructor |
|---|---|---|
| `github.com/vormialabs/vormia-go-driver-chi` **v1.1.0+** | `contract.Router` | `chi.New()` |
| `github.com/vormialabs/vormia-go-driver-sqlite` | `contract.Database` | `sqlite.Open(sqlite.Config{Path: ...})` |
| `github.com/vormialabs/vormia-go-driver-postgresql` | `contract.Database` | `postgres.Open(postgres.Config{Host, User, Database, ...})` |
| `github.com/vormialabs/vormia-go-driver-mysql` | `contract.Database` | `mysql.Open(mysql.Config{...})` |
| `github.com/vormialabs/vormia-go-driver-redis` | `contract.Cache` | `redis.Open(redis.Config{Addr: ...})` |

## 2. Architectural Invariants (never violate)

1. **`contract` imports only the Go standard library.** Never add a third-party import to `contract/contract.go`.
2. **Framework production code (`app`, `contract`, `db`, `migrate`, `seed`, `cache`) never imports a concrete driver.** Drivers appear only in `_test.go` files and in end-user applications. Verify with `go list -deps ./db ./migrate ./seed ./cache` (must not list `vormia-go-driver-*`).
3. **Drivers never import vormia-go.** They satisfy contracts structurally. `RouteInfo` is a type alias so chi can implement `Routes()` without importing this module. When writing a driver, do not add `vormia-go` to its `go.mod`.
4. **Open connections via app-registered `db.Opener` / `cache.Opener`s**, not by importing drivers inside `db` or `cache`. The app's closure calls `sqlite.Open` / `postgres.Open` / `redis.Open`.
5. **Contract changes are breaking changes** for every driver. Do not add, remove, or re-sign a contract method casually — flag it to the user first. (`Routes()` landed in v1.2.0; `Flush` is still absent.)
6. Router grouping (`Group`/`Route`) is deliberately **not** in `contract.Router`. Do not add it; it cannot be expressed portably.
7. **Do not move `DB_<NAME>_*` or `CACHE_<NAME>_*` connection semantics into vormia-go-core.** Core only provides generic `Prefixed`; this module owns the registry rules.

## 3. Canonical Usage Patterns

### Bootstrap an application (direct open)

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

### Bootstrap with the connection registry

```go
reg := db.New(cfg)
reg.RegisterOpener("sqlite", func(c db.ConnConfig) (contract.Database, error) {
	return sqlite.Open(sqlite.Config{Path: c.Path})
})
reg.RegisterOpener("postgres", func(c db.ConnConfig) (contract.Database, error) {
	port, _ := strconv.Atoi(c.Port)
	return postgres.Open(postgres.Config{
		Host: c.Host, Port: port, User: c.User,
		Password: c.Password, Database: c.Database, SSLMode: c.SSLMode,
	})
})

defaultDB, err := reg.Connection("") // DB_CONNECTION or "default"
if err != nil { /* handle */ }
k := app.New(chi.New(), app.WithDB(defaultDB))
// secondary, err := reg.Connection("mysql2")
```

Config keys:

| Connection | Keys |
|---|---|
| `default` | Bare `DB_*` |
| Named `X` | `DB_<UPPER(X)>_*` via `cfg.Prefixed` |
| Default name | `DB_CONNECTION` (fallback `"default"`) |

### Run migrations

```go
m := migrate.New(database, os.DirFS("database/migrations"))
run, err := m.Up(ctx)
rolled, err := m.Rollback(ctx, 0) // latest batch; use steps > 0 for N newest
ver, err := m.Version(ctx)
rows, err := m.Status(ctx) // []StatusRow{Version, Applied, Batch} in apply order
```

Files: `<version>.up.sql` / `<version>.down.sql`. Tracking table: `schema_migrations`. Prefer one statement per file for MySQL (DDL is not transactional; multi-statement needs DSN `multiStatements=true`). `Status` does not run SQL files — it only reports disk vs applied.

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

### URL parameters (not portable yet)

There is **no portable `Param` yet**. If the app needs route parameters, use the concrete router's mechanism in the handler (e.g. `chi.URLParam(req, "id")` via `github.com/go-chi/chi/v5`) and note that this couples the handler to chi. Prefer query strings (`req.URL.Query().Get("id")`) when portability matters.

### JSON responses (no helpers yet)

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

### Real database / migrations in tests

```go
db, err := sqlite.Open(sqlite.Config{Path: ":memory:"})
src := fstest.MapFS{
	"20260101_create_users.up.sql":   {Data: []byte(`CREATE TABLE users (...)`)},
	"20260101_create_users.down.sql": {Data: []byte(`DROP TABLE users`)},
}
m := migrate.New(db, src)
```

Use the sqlite driver with an in-memory DB — it is CGo-free (`modernc.org/sqlite`) and runs anywhere.

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
| Importing a driver inside `app/`, `contract/`, `db/`, `migrate/`, `seed/`, or `cache/` production code | Drivers only in `_test.go` and user applications |
| Opening DBs/caches inside registries by importing drivers | App registers `Opener` closures; registry only looks them up |
| Putting `DB_<NAME>_*` / `CACHE_<NAME>_*` rules into vormia-go-core | Core stays generic (`Prefixed`); registries own the convention |
| Calling `k.Use(...)` after `k.Routes(...)` | Middleware first; chi panics otherwise |
| Treating a cache miss as an error | Check `Get`'s `bool`, not `err` |
| Writing `$1` placeholders directly | Write `?` and pass through `k.DB.Rebind(...)` |
| Passing `context.Background()` in handlers | Pass `req.Context()` so client disconnects cancel work |
| Calling `k.Run` in unit tests | Use `k.Router.ServeHTTP` with `httptest` |
| Inventing `Cache.Flush`, CLI Group C commands, ORM, or JSON helpers as if they exist | Flush / CLI wrappers not in v1.2.0; note the gap |
| Assuming MySQL DDL / Wipe rolls back with `BeginTx` | MySQL auto-commits DDL; Wipe is non-atomic on MySQL |
| Manual `defer db.Close()` alongside `k.Run` | `Run` closes attached drivers on shutdown; registries need their own `Close` if you keep one |
| Adding fields to `Kernel` config via new `New` parameters | Add a functional `Option` (`WithX`) instead |
| Using chi older than v1.1.0 with vormia-go v1.2.0 | Upgrade chi — `Routes()` is required on `contract.Router` |

## 7. Repository Conventions (when editing vormia-go itself)

- Keep packages focused: `contract` and `app` are still one file each; `db`, `migrate`, `seed`, and `cache` follow the same style until size demands a split.
- Every exported identifier gets a doc comment; comments explain intent and trade-offs, not mechanics.
- Tests live in external test packages (`package db_test`, `package migrate_test`, `package seed_test`, `package cache_test`, `package app_test`) and exercise real drivers or stubs end-to-end rather than mocks.
- New optional kernel dependencies follow the existing pattern: field on `Kernel` (interface type, `// may be nil` comment), a `WithX` option, and closing in `closeDrivers` if it has a `Close`.
- Anything that would expand the contracts, import a driver in production code, or add a framework dependency is an architectural decision — surface it to the user before implementing.
