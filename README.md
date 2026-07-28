# vormia-go

Vormia Go — a modular, Laravel-inspired application framework for Go that wires swappable router, database, and cache drivers behind stable contracts.

**Status:** Slice 1 (kernel + contracts). The framework spine is in place: boot an app, wire drivers, register routes, serve, and shut down gracefully. ORM, validation, auth, and CLI scaffolding come in later slices.

## Install

```bash
go get github.com/vormialabs/vormia-go
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

## Packages

| Package | Purpose |
|---------|---------|
| [`contract`](contract/contract.go) | Canonical `Router`, `Database`, and `Cache` interfaces (stdlib only) |
| [`app`](app/kernel.go) | `Kernel` — wire drivers, register routes, serve, graceful shutdown |

### Kernel API

| Method | Purpose |
|--------|---------|
| `New(router, opts...)` | Build a kernel around a router; optional `WithDB`, `WithCache` |
| `Use(mw...)` | Register global middleware (before routes) |
| `Routes(fn)` | Register routes via a callback that receives `contract.Router` |
| `Run(addr)` | Start the server; block until SIGINT/SIGTERM; shut down gracefully |

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

The end-to-end test uses real chi and sqlite drivers with an in-memory database — no Docker, no external services.

```bash
go test -v ./...
```

## Roadmap

- **Slice 2** — HTTP ergonomics: JSON/error helpers, request context, router-agnostic `Param`
- **Slice 3+** — ORM/query builder, validation, auth, CLI scaffolding, DI container integration

## License

MIT — see [LICENSE](LICENSE).
