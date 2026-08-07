# vormia-go

Vormia Go — a modular, Laravel-inspired application framework for Go that wires swappable router, database, and cache drivers behind stable contracts.

**Version:** v1.1.0 — connection registry + migration engine on top of the Slice 1 kernel. ORM, validation, auth, and CLI scaffolding come in later slices.

## Install

```bash
go get github.com/vormialabs/vormia-go@v1.1.0
```

Requires Go 1.26+.

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
vormia-go ──imports──▶ contract                     (never a concrete driver)
driver-* ──imports──▶ (nothing from vormia-go)      (structural satisfaction)
```

Drivers satisfy the canonical interfaces without importing the framework. Your app imports both the framework and whichever drivers it needs, then passes them to the kernel as interface values.

Named connections use the same rule: the app registers an **opener** per driver it imported; `db.Registry` looks up the opener from config (`DB_DRIVER` / `DB_<NAME>_DRIVER`) and never imports a driver itself.

## Packages

| Package | Purpose |
|---------|---------|
| [`contract`](contract/contract.go) | Canonical `Router`, `Database`, and `Cache` interfaces (stdlib only) |
| [`app`](app/kernel.go) | `Kernel` — wire drivers, register routes, serve, graceful shutdown |
| [`db`](db/registry.go) | Named connection registry — resolve config, open via registered openers, cache live connections |
| [`migrate`](migrate/migrate.go) | SQL migration engine against any `contract.Database` (`.up.sql` / `.down.sql`) |

### Kernel API

| Method | Purpose |
|--------|---------|
| `New(router, opts...)` | Build a kernel around a router; optional `WithDB`, `WithCache` |
| `Use(mw...)` | Register global middleware (before routes) |
| `Routes(fn)` | Register routes via a callback that receives `contract.Router` |
| `Run(addr)` | Start the server; block until SIGINT/SIGTERM; shut down gracefully |

### Connection registry (`db`)

Resolve named connections from the config convention locked in vormia-go-core (`DB_*` for `default`, `DB_<NAME>_*` for others). The app registers one opener per driver:

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
```

| Method | Purpose |
|--------|---------|
| `New(cfg)` | Bind a registry to a config source |
| `RegisterOpener(driver, opener)` | Wire a driver name to an opener (app-owned) |
| `Default()` | Connection name from `DB_CONNECTION` (fallback `"default"`) |
| `Resolve(name)` | Build `ConnConfig` without opening |
| `Connection(name)` | Resolve, open once (cached), return `contract.Database` |
| `Close()` | Close every live connection |

### Migrations (`migrate`)

Apply and roll back SQL files against any `contract.Database`. Pass an `fs.FS` (e.g. `os.DirFS("database/migrations")` or `fstest.MapFS` in tests).

| Method | Purpose |
|--------|---------|
| `New(db, src)` | Build a migrator; `src` holds `<version>.up.sql` / `<version>.down.sql` |
| `Up(ctx)` | Apply all pending migrations in one new batch |
| `Rollback(ctx, steps)` | `steps <= 0` rolls back the latest batch; otherwise that many, newest first |
| `Reset(ctx)` | Roll back everything |
| `Version(ctx)` | Latest applied version, or `""` |

## Contracts

All three interfaces live in `contract` and depend only on the standard library.

- **Router** — HTTP verbs, middleware, `Serve` / `Shutdown`, `ServeHTTP`
- **Database** — `QueryContext`, `QueryRowContext`, `ExecContext`, `BeginTx`, `PingContext`, `Close`, `Rebind`
- **Cache** — `Get`, `Set`, `Delete`, `Exists`, `Ping`, `Close`

## Drivers

| Driver | Contract | Repository |
|--------|----------|------------|
| Chi | `Router` | [vormia-go-driver-chi](https://github.com/vormialabs/vormia-go-driver-chi) |
| SQLite | `Database` | [vormia-go-driver-sqlite](https://github.com/vormialabs/vormia-go-driver-sqlite) |
| PostgreSQL | `Database` | [vormia-go-driver-postgresql](https://github.com/vormialabs/vormia-go-driver-postgresql) |
| MySQL | `Database` | [vormia-go-driver-mysql](https://github.com/vormialabs/vormia-go-driver-mysql) |
| Redis | `Cache` | [vormia-go-driver-redis](https://github.com/vormialabs/vormia-go-driver-redis) |

## Testing

Tests use real chi and sqlite drivers with an in-memory database and `fstest.MapFS` for migrations — no Docker, no external services.

```bash
go test -v ./...
```

## Roadmap

- **Next** — CLI `migrate*` / `db:*` commands and `--database` flag (thin front-ends over `db` + `migrate`)
- **Later** — HTTP ergonomics, ORM/query builder, validation, auth, DI container integration

## License

MIT — see [LICENSE](LICENSE).
