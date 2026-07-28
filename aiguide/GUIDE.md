# How vormia-go Works

A walkthrough of the framework's design, the two packages that make it up, and the lifecycle of an app built on it. Read the [README](../README.md) first for install and quick-start; this guide explains the *why* and *how* underneath.

**Version:** v1.0.0 (Slice 1 — kernel + contracts)

---

## 1. The Big Idea

vormia-go is a **Laravel-inspired application spine for Go**. Like Laravel, it gives you a kernel that boots the app, wires services (database, cache, router), registers routes, serves HTTP, and shuts down gracefully.

Unlike Laravel, it does **not** ship any concrete implementations. The framework only knows about *interfaces*. Your application chooses the concrete drivers (chi, PostgreSQL, Redis, ...) and hands them to the kernel.

This is the whole dependency picture:

```
your app  ──imports──▶  driver-chi, driver-postgresql, ...   (concretes YOU chose)
your app  ──imports──▶  vormia-go                             (the framework)
vormia-go ──imports──▶  contract                              (interfaces only)
driver-*  ──imports──▶  nothing from vormia-go                (structural satisfaction)
```

Two properties fall out of this:

1. **Swappability** — change `postgres.Open(...)` to `sqlite.Open(...)` in `main()` and nothing else in your codebase changes, because everything downstream only sees `contract.Database`.
2. **No import cycles, no coupling** — drivers never import the framework. They just happen to have the right method sets. This works because Go interfaces are satisfied *structurally* (see [Structural typing](#5-the-go-concepts-that-make-this-work) below).

---

## 2. Package: `contract` — The Canonical Interfaces

Everything lives in one file: [`contract/contract.go`](../contract/contract.go). It imports **only the standard library** — that is the rule that lets drivers satisfy these interfaces without ever importing vormia-go.

### `Router`

The portable surface of an HTTP router:

| Method | Purpose |
|--------|---------|
| `Get/Post/Put/Patch/Delete/Head/Options(pattern, handlerFunc)` | Register a route per HTTP verb |
| `Use(mw ...func(http.Handler) http.Handler)` | Add global middleware (standard Go middleware shape) |
| `Serve(addr string) error` | Bind and listen — blocks until the server stops |
| `Shutdown(ctx) error` | Graceful stop: finish in-flight requests, then exit |
| `ServeHTTP(w, r)` | Makes the router itself an `http.Handler` — this is what enables in-process testing with `httptest` |

Deliberate omission: **no `Group` / `Route` methods**. Sub-router grouping is concrete-typed on each driver (chi's groups return chi types, gin's return gin types), so it can't be expressed portably. Only the stdlib-shaped surface belongs in the contract.

### `Database`

The portable surface of a SQL database:

| Method | Purpose |
|--------|---------|
| `QueryContext` / `QueryRowContext` / `ExecContext` | The three standard query operations, context-aware |
| `BeginTx` | Start a transaction |
| `PingContext` | Health check |
| `Close` | Release the connection pool |
| `Rebind(query) string` | Translate `?` placeholders to the driver's style (e.g. `$1` for Postgres) |

Design note: a driver gets everything except `Rebind` **for free** by embedding `*sql.DB`. `Rebind` exists because placeholder syntax is the one thing `database/sql` does not abstract — SQLite/MySQL use `?`, PostgreSQL uses `$1, $2, ...`. Write queries with `?` and pass them through `Rebind` and they run on any driver.

### `Cache`

A minimal byte-oriented cache:

| Method | Purpose |
|--------|---------|
| `Get(ctx, key) ([]byte, bool, error)` | The `bool` distinguishes "key missing" from "error" — a miss is not an error |
| `Set(ctx, key, value, ttl)` | Store with expiry |
| `Delete` / `Exists` / `Ping` / `Close` | The rest of the essentials |

Values are `[]byte`, not `any` — serialization (JSON, gob, ...) is the caller's decision, not the cache's.

---

## 3. Package: `app` — The Kernel

Also one file: [`app/kernel.go`](../app/kernel.go). The `Kernel` struct holds the three drivers, all as contract interfaces:

```17:21:app/kernel.go
type Kernel struct {
	Router contract.Router
	DB     contract.Database // may be nil
	Cache  contract.Cache    // may be nil
}
```

Router is required (it's the positional argument to `New`); DB and Cache are optional and attached with **functional options**:

```go
k := app.New(chi.New())                                  // router only
k := app.New(chi.New(), app.WithDB(db))                  // + database
k := app.New(chi.New(), app.WithDB(db), app.WithCache(c)) // + cache
```

`Option` is just `func(*Kernel)`; `New` applies each one to the kernel it builds. This is the standard Go pattern for optional configuration — new options can be added later without breaking `New`'s signature.

### The four kernel methods

**`Use(mw...)`** forwards middleware straight to the router. Call it *before* `Routes` — most routers (chi included) require middleware to be registered before any route.

**`Routes(fn)`** is intentionally thin: it hands you the `contract.Router` and you register routes on it. The callback shape keeps route registration in one visible block and mirrors Laravel's route files.

**`Run(addr)`** is the lifecycle heart. Step by step:

```53:74:app/kernel.go
func (k *Kernel) Run(addr string) error {
	serveErr := make(chan error, 1)
	go func() { serveErr <- k.Router.Serve(addr) }()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serveErr:
		// Server stopped on its own (e.g. failed to bind the port).
		return err
	case <-stop:
		// Interrupt received — fall through to graceful shutdown.
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	shutdownErr := k.Router.Shutdown(ctx)
	k.closeDrivers()
	return shutdownErr
}
```

1. The server starts in a **goroutine** so `Run` can keep listening for signals; its result goes into the `serveErr` channel.
2. `signal.Notify` subscribes to SIGINT (Ctrl-C) and SIGTERM (what Docker/Kubernetes send on stop).
3. The `select` races the two channels — whichever event happens first wins:
   - **Server dies on its own** (port already in use, etc.) → return that error immediately.
   - **Signal arrives** → fall through to graceful shutdown.
4. Shutdown gets a **10-second budget** via `context.WithTimeout`: in-flight requests may finish, but the process will not hang forever.
5. `closeDrivers` closes DB and Cache *after* the router stops — handlers may use them until the last request completes, so this ordering matters.

**`closeDrivers`** nil-checks each optional driver and ignores close errors — at shutdown there is nothing useful left to do with them.

### Full lifecycle of a vormia-go app

```
main()
  │
  ├─ open drivers          postgres.Open(...), redis.Open(...)
  ├─ app.New(router, ...)  build the kernel, attach drivers
  ├─ k.Use(...)            global middleware (optional, before routes)
  ├─ k.Routes(...)         register handlers; capture k to reach k.DB / k.Cache
  └─ k.Run(":8080")        serve ── blocks here ──▶ SIGINT/SIGTERM
                                                        │
                                                        ├─ router.Shutdown (≤10s)
                                                        ├─ db.Close, cache.Close
                                                        └─ return
```

Handlers reach the drivers by **closing over the kernel**:

```go
k.Routes(func(r contract.Router) {
	r.Get("/users", func(w http.ResponseWriter, req *http.Request) {
		rows, err := k.DB.QueryContext(req.Context(), k.DB.Rebind(
			`SELECT name FROM users WHERE active = ?`), true)
		// ...
	})
})
```

Note `req.Context()` being passed to the query — if the client disconnects, the query is cancelled too.

---

## 4. How Drivers Fit (and How to Write One)

A driver is an external module that structurally satisfies one contract interface. It never imports vormia-go. The shape, using the sqlite driver as the model:

```go
package sqlite // module github.com/vormialabs/vormia-go-driver-sqlite

type DB struct {
	*sql.DB // embed: QueryContext, ExecContext, BeginTx, ... come free
}

func Open(cfg Config) (*DB, error) { /* open, ping, return */ }

func (d *DB) Rebind(q string) string { return q } // sqlite uses ? natively
```

The framework's own test then pins the architecture with compile-time assertions:

```19:22:app/kernel_test.go
var (
	_ contract.Router   = (*chi.Router)(nil)
	_ contract.Database = (*sqlite.DB)(nil)
)
```

If a driver drifts from the contract, this block stops compiling — the check costs nothing at runtime.

To write a new driver (say, a memory cache): create a new module, implement the six `contract.Cache` methods against your backend, and add nothing else. Any vormia-go app can then use it via `app.WithCache(memory.New())`.

### Why does `go.mod` list chi and sqlite if the framework never imports drivers?

They are **test-only dependencies**. `app/kernel_test.go` is an end-to-end test that boots the kernel with the real chi router and a real in-memory SQLite database, drives a request through `ServeHTTP` with `httptest` (no network, no Docker), and asserts the response came from the database. Production code under `app/` and `contract/` imports no driver — the constraint holds where it matters.

---

## 5. The Go Concepts That Make This Work

If you're coming from PHP/Laravel, these are the language features doing the heavy lifting — with the official docs for each:

| Concept | Role here | Docs |
|---------|-----------|------|
| **Structural (implicit) interfaces** | Drivers satisfy contracts without importing them — no `implements` keyword exists in Go | [Go Tour: interfaces are implemented implicitly](https://go.dev/tour/methods/10) |
| **Struct embedding** | A driver embedding `*sql.DB` inherits its whole method set — that's most of `contract.Database` for free | [Effective Go: embedding](https://go.dev/doc/effective_go#embedding) |
| **Functional options** | `WithDB` / `WithCache` — extensible optional config without breaking `New` | [Dave Cheney: functional options](https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis) |
| **Goroutines + channels + `select`** | `Run` races "server crashed" against "signal received" | [Go Tour: concurrency](https://go.dev/tour/concurrency/1) |
| **`context.Context`** | Request cancellation flows from HTTP request → your handler → the DB query; also bounds the shutdown to 10s | [Go blog: context](https://go.dev/blog/context) |
| **`os/signal`** | Turning SIGINT/SIGTERM into a channel receive | [pkg.go.dev/os/signal](https://pkg.go.dev/os/signal) |
| **`net/http` + `http.Handler`** | The universal HTTP currency — middleware is `func(http.Handler) http.Handler`, and `ServeHTTP` makes the router testable in-process | [pkg.go.dev/net/http](https://pkg.go.dev/net/http) |
| **`database/sql`** | The stdlib DB abstraction the `Database` contract is shaped around | [Go: accessing databases](https://go.dev/doc/database/) |
| **`httptest`** | In-process request/response testing without a network listener | [pkg.go.dev/net/http/httptest](https://pkg.go.dev/net/http/httptest) |

Laravel translation table, roughly:

| Laravel | vormia-go |
|---------|-----------|
| `App\Http\Kernel` + `bootstrap/app.php` | `app.Kernel` + `app.New(...)` |
| Service container binding an interface to a concrete | You pass the concrete into `New` / `WithDB` as an interface value |
| `routes/web.php` | The `k.Routes(func(r contract.Router) { ... })` callback |
| Global middleware in `Kernel::$middleware` | `k.Use(...)` |
| `php artisan serve` / FPM lifecycle | `k.Run(":8080")` with built-in graceful shutdown |
| Database manager (`DB::`) | `k.DB` (`contract.Database`) |
| Cache manager (`Cache::`) | `k.Cache` (`contract.Cache`) |

---

## 6. Current Scope and What's Next

This is **Slice 1**: the spine only. What exists today is the kernel, the three contracts, five external drivers, and the end-to-end test proving they compose.

Not here yet (per the roadmap):

- **Slice 2** — HTTP ergonomics: JSON/error response helpers, request context helpers, a router-agnostic `Param` for URL parameters
- **Slice 3+** — ORM/query builder, validation, auth, CLI scaffolding, DI container integration

Until then you work close to the stdlib: raw SQL through `k.DB`, manual `w.Write` / `http.Error` in handlers, and driver-specific route parameter extraction.

---

## 7. Running the Tests

```bash
go test -v ./...
```

The suite needs no external services — chi is pure Go and the sqlite driver (`modernc.org/sqlite`) is a CGo-free port, so an in-memory database runs anywhere Go runs.
