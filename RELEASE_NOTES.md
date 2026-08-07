# Release Notes

## v1.1.0 — Connection Registry + Migration Engine (2026-08-07)

Vormia Go **v1.1.0** adds two engine-agnostic packages on top of the Slice 1 kernel: a named connection registry and a SQL migration engine. Both sit on `contract.Database` and import no concrete drivers.

### What this release does

**Connection registry (`db`).** Resolve connections from the config convention in vormia-go-core (`DB_*` for `default`, `DB_<NAME>_*` for named connections). The app registers one `Opener` per driver it imports; the registry looks up the opener by `DRIVER` and caches live connections. The framework still never imports `driver-postgresql`, `driver-mysql`, or `driver-sqlite`.

**Migration engine (`migrate`).** Apply and roll back `<version>.up.sql` / `<version>.down.sql` files from any `fs.FS`. Tracks applied versions in a portable `schema_migrations` table. Supports `Up`, `Rollback` (by batch or step count), `Reset`, and `Version`. Placeholder style goes through the driver's `Rebind`.

**Dependency.** Production code in `db` depends on `vormia-go-core/config` (`GetString`, `Prefixed`). Drivers remain test-only dependencies (SQLite in-memory).

### Install

```bash
go get github.com/vormialabs/vormia-go@v1.1.0
```

Requires Go 1.26+.

### What's not in this release

CLI `migrate*` / `db:*` commands and starterkit opener wiring land in the next step. ORM, validation, auth, and HTTP ergonomics remain later work.

---

## v1.0.1 — Official Slice 1 Release (2026-08-04)

Vormia Go **v1.0.1** is the official Slice 1 release. It delivers the framework's spine: you can boot an application, wire in your chosen drivers, register routes and middleware, serve HTTP, and shut down gracefully — all without the framework ever depending on a concrete router, database, or cache implementation.

This tag supersedes `v1.0.0` as the recommended install target. The feature set is Slice 1 (kernel + contracts); v1.0.1 packages that spine with the release notes and documentation refinements that landed after the initial tag.

### What this release does

**Defines the contracts.** The `contract` package contains three canonical interfaces, built on the standard library only:

- `Router` — HTTP verb registration (`Get`, `Post`, etc.), middleware, `Serve` / `Shutdown`, and `ServeHTTP`.
- `Database` — `QueryContext`, `QueryRowContext`, `ExecContext`, `BeginTx`, `PingContext`, `Close`, and `Rebind` for cross-dialect placeholder handling.
- `Cache` — `Get`, `Set`, `Delete`, `Exists`, `Ping`, and `Close`.

**Provides the kernel.** The `app` package exposes `Kernel`, the piece that ties an application together:

- `New(router, opts...)` builds a kernel around any `contract.Router`, with `WithDB` and `WithCache` options for attaching a database and cache.
- `Use(mw...)` registers global middleware before routes.
- `Routes(fn)` registers routes through a callback that receives the `contract.Router`, keeping your route definitions driver-agnostic.
- `Run(addr)` starts the server, blocks until SIGINT/SIGTERM, and shuts down gracefully.

**Inverts the dependency direction.** Your application imports the framework *and* the concrete drivers it wants, then hands them to the kernel as interface values. The framework only ever imports `contract`, and drivers satisfy the interfaces structurally without importing the framework at all. The practical effect: swap PostgreSQL for MySQL or SQLite and nothing else in your code changes.

### First-party drivers

Five drivers ship as separate modules, ready to plug in:

| Driver | Contract | Repository |
|--------|----------|------------|
| Chi | `Router` | [vormia-go-driver-chi](https://github.com/vormialabs/vormia-go-driver-chi) |
| SQLite | `Database` | [vormia-go-driver-sqlite](https://github.com/vormialabs/vormia-go-driver-sqlite) |
| PostgreSQL | `Database` | [vormia-go-driver-postgresql](https://github.com/vormialabs/vormia-go-driver-postgresql) |
| MySQL | `Database` | [vormia-go-driver-mysql](https://github.com/vormialabs/vormia-go-driver-mysql) |
| Redis | `Cache` | [vormia-go-driver-redis](https://github.com/vormialabs/vormia-go-driver-redis) |

### Testing

The release includes an end-to-end test that runs the kernel with the real chi and sqlite drivers against an in-memory database — no Docker or external services needed (`go test -v ./...`).

### Install

```bash
go get github.com/vormialabs/vormia-go@v1.0.1
```

Requires Go 1.26+.

### What's not in this release

ORM/query builder, validation, auth, and CLI scaffolding are planned for later slices. Slice 2 will focus on HTTP ergonomics: JSON/error helpers, request context, and a router-agnostic `Param`.

---

## v1.0.0 — Slice 1: Kernel + Contracts (2026-07-28)

Initial Slice 1 tag. Prefer **v1.0.1** for new installs.
