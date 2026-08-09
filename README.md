# vormia-go

Vormia Go — a modular, Laravel-inspired application framework for Go that wires swappable router, database, and cache drivers behind stable contracts.

**Version:** v1.2.1 — kernel + contracts (including `Router.Routes()`), `db` registry + `Wipe`, `migrate`, SQL `seed`, and `cache` registry. Pair with [vormia-go-driver-chi](https://github.com/vormialabs/vormia-go-driver-chi) **v1.1.0+** for `Routes()`. ORM, validation, auth, and CLI wrappers come later.

Human walkthrough: [aiguide/GUIDE.md](aiguide/GUIDE.md). AI editor rules: [aiguide/CURSOR_CODEX_MCP_GUIDE.md](aiguide/CURSOR_CODEX_MCP_GUIDE.md). Changelog: [RELEASE_NOTES.md](RELEASE_NOTES.md).

## Install

```bash
go get github.com/vormialabs/vormia-go@v1.2.1
```

Requires Go 1.26+. Named connections use [`vormia-go-core/config`](https://github.com/vormialabs/vormia-go-core) (`GetString`, `Prefixed`).

## Quick start

Your application picks the concrete drivers. The framework never imports them — only the interfaces in `contract`.

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
	db, _ := postgres.Open(postgres.Config{Host: "localhost", User: "app", Database: "app"})
	cache, _ := redis.Open(redis.Config{Addr: "localhost:6379"})

	k := app.New(chi.New(), app.WithDB(db), app.WithCache(cache))

	k.Routes(func(r contract.Router) {
		r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		})
	})

	_ = k.Run(":8080")
}
```

Swap `postgres` for `mysql` or `sqlite` and nothing else changes.

## Architecture

```
app  ──imports──▶ driver-chi, driver-postgresql   (the concretes you chose)
app  ──imports──▶ vormia-go                         (the framework)
vormia-go ──imports──▶ contract + core/config       (never a concrete driver)
driver-* ──imports──▶ (nothing from vormia-go)      (structural satisfaction)
```

Drivers satisfy the canonical interfaces without importing the framework. Your app imports both the framework and whichever drivers it needs, then passes them to the kernel as interface values.

Named connections keep the same rule: the app registers an **opener** per driver it imported; `db.Registry` / `cache.Registry` look up the opener from config and never import a driver themselves.

## Packages

| Package | Purpose |
|---------|---------|
| [`contract`](contract/contract.go) | Canonical `Router` (incl. `Routes()`), `Database`, and `Cache` interfaces (stdlib only) |
| [`app`](app/kernel.go) | `Kernel` — wire drivers, register routes, serve, graceful shutdown |
| [`db`](db/registry.go) | Named connection registry + [`Wipe`](db/wipe.go) |
| [`migrate`](migrate/migrate.go) | SQL migration engine against any `contract.Database` |
| [`seed`](seed/seed.go) | Run `*.sql` seeders from an `fs.FS` (no tracking table) |
| [`cache`](cache/registry.go) | Named cache connection registry (`CACHE_*` / `CACHE_<NAME>_*`) |

### Kernel API

| Method | Purpose |
|--------|---------|
| `New(router, opts...)` | Build a kernel around a router; optional `WithDB`, `WithCache` |
| `Use(mw...)` | Register global middleware (before routes) |
| `Routes(fn)` | Register routes via a callback that receives `contract.Router` |
| `Run(addr)` | Start the server; block until SIGINT/SIGTERM; shut down gracefully |

### Connection registry (`db`)

Config convention (owned by vormia-go, resolved via core `Prefixed`):

| Connection | Keys |
|------------|------|
| `default` | Bare `DB_*` (`DB_DRIVER`, `DB_HOST`, `DB_PATH`, …) |
| Named (e.g. `mysql2`) | `DB_MYSQL2_*` (`DB_MYSQL2_DRIVER`, `DB_MYSQL2_HOST`, …) |
| Which name is default | `DB_CONNECTION` (fallback `"default"`) |

The app registers one opener per driver:

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

defaultDB, _ := reg.Connection("") // uses DB_CONNECTION, or "default"
k := app.New(chi.New(), app.WithDB(defaultDB))
```

| Method / func | Purpose |
|---------------|---------|
| `New(cfg)` | Bind a registry to a config source |
| `RegisterOpener(driver, opener)` | Wire a driver name to an opener (app-owned) |
| `Default()` | Connection name from `DB_CONNECTION` (fallback `"default"`) |
| `Resolve(name)` | Build `ConnConfig` without opening |
| `Connection(name)` | Resolve, open once (cached), return `contract.Database` |
| `Close()` | Close every live connection |
| `Wipe(ctx, conn, driver)` | Drop all user tables (dialect-specific; destructive) |

`Wipe` needs the driver name because the `Database` contract hides the engine. MySQL wipes are **non-atomic** (FK checks toggled around drops).

### Migrations (`migrate`)

Apply and roll back SQL files against any `contract.Database`. Pass an `fs.FS` (e.g. `os.DirFS("database/migrations")` or `fstest.MapFS` in tests). Files are named `<version>.up.sql` / `<version>.down.sql`; applied versions live in `schema_migrations`.

```go
m := migrate.New(database, os.DirFS("database/migrations"))
run, err := m.Up(ctx)           // pending migrations, one new batch
rolled, err := m.Rollback(ctx, 0) // latest batch (steps <= 0); or N newest
rows, err := m.Status(ctx)      // every on-disk version + applied/batch
```

| Method | Purpose |
|--------|---------|
| `New(db, src)` | Build a migrator |
| `Up(ctx)` | Apply all pending migrations in one new batch |
| `Rollback(ctx, steps)` | `steps <= 0` = latest batch; otherwise that many, newest first |
| `Reset(ctx)` | Roll back everything |
| `Version(ctx)` | Latest applied version, or `""` |
| `Status(ctx)` | Every on-disk migration with `Applied` / `Batch` (`[]StatusRow`) |

### Seeders (`seed`)

Run every `*.sql` file in an `fs.FS` in filename order. No tracking table — authors keep seeders idempotent.

```go
ran, err := seed.Run(ctx, database, os.DirFS("database/seeders"))
```

### Cache registry (`cache`)

Same opener pattern as `db`, with `CACHE_*` / `CACHE_<NAME>_*` and `CACHE_CONNECTION`.

| Method | Purpose |
|--------|---------|
| `New(cfg)` | Bind a registry to a config source |
| `RegisterOpener(driver, opener)` | Wire a cache driver name to an opener |
| `Default()` / `Resolve` / `Connection` / `Close` | Same shape as `db.Registry` |

There is **no** `Flush` on `contract.Cache` yet — a future release may add it for full cache clears.

### Route introspection

`contract.Router` includes `Routes() []RouteInfo` (`Method`, `Pattern`). Use [vormia-go-driver-chi](https://github.com/vormialabs/vormia-go-driver-chi) **v1.1.0+**.

## Contracts

All three interfaces live in `contract` and depend only on the standard library.

- **Router** — HTTP verbs, middleware, `Serve` / `Shutdown`, `ServeHTTP`, `Routes()`
- **Database** — `QueryContext`, `QueryRowContext`, `ExecContext`, `BeginTx`, `PingContext`, `Close`, `Rebind`
- **Cache** — `Get`, `Set`, `Delete`, `Exists`, `Ping`, `Close` (no `Flush` yet)

## Drivers

| Driver | Contract | Repository |
|--------|----------|------------|
| Chi | `Router` | [vormia-go-driver-chi](https://github.com/vormialabs/vormia-go-driver-chi) (**v1.1.0+** for `Routes()`) |
| SQLite | `Database` | [vormia-go-driver-sqlite](https://github.com/vormialabs/vormia-go-driver-sqlite) |
| PostgreSQL | `Database` | [vormia-go-driver-postgresql](https://github.com/vormialabs/vormia-go-driver-postgresql) |
| MySQL | `Database` | [vormia-go-driver-mysql](https://github.com/vormialabs/vormia-go-driver-mysql) |
| Redis | `Cache` | [vormia-go-driver-redis](https://github.com/vormialabs/vormia-go-driver-redis) |

## Testing

Default suite uses real chi and sqlite drivers with an in-memory database and `fstest.MapFS` for migrations/seeders — no Docker required. Postgres/MySQL wipe integration tests skip unless `PG_TEST_HOST` / `MYSQL_TEST_HOST` are set (same env vars as the driver repos).

```bash
go test -v ./...
```

Boundary check: `go list -deps ./db ./migrate ./seed ./cache` must not list any `vormia-go-driver-*` module.

## Roadmap

- **Next** — CLI Group C wrappers (`db:wipe`, `migrate:fresh`, `db:seed`, `cache:ping` / `cache:forget`, `route:list`) in vormia-go-cli
- **Later** — `Cache.Flush` (for `cache:clear`), HTTP ergonomics, ORM/query builder, validation, auth, DI container integration

## License

MIT — see [LICENSE](LICENSE).
