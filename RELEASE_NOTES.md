# Release Notes

## v1.2.1 — Gated Wipe Integration Tests (2026-08-09)

Vormia Go **v1.2.1** is the recommended install tag. Same library surface as v1.2.0, plus Postgres/MySQL wipe integration tests gated on `PG_TEST_HOST` / `MYSQL_TEST_HOST` (they skip cleanly when unset).

**Pair with** [vormia-go-driver-chi](https://github.com/vormialabs/vormia-go-driver-chi) **v1.1.0+** — older chi builds do not implement `Routes()`.

### What this release includes

Everything from **v1.2.0** (`db.Wipe`, `seed.Run`, `cache.Registry`, `Router.Routes()`, registry, migrator including `Status`, kernel, contracts), plus:

**Gated wipe tests** — `TestWipePostgres` / `TestWipeMySQL` exercise the dialect paths against real engines when the same env vars as the driver repos are set. Default `go test ./...` still needs no Docker.

### Install

```bash
go get github.com/vormialabs/vormia-go@v1.2.1
go get github.com/vormialabs/vormia-go-driver-chi@v1.1.0
```

Requires Go 1.26+.

### Testing

```bash
go test -v ./...
```

Boundary check: `go list -deps ./db ./migrate ./seed ./cache` must not list any `vormia-go-driver-*` module.

### What's not in this release

CLI Group C command wrappers (`db:wipe`, `migrate:fresh`, …) live in vormia-go-cli (separate plan). `Cache.Flush` / `cache:clear`, ORM, validation, auth, and HTTP ergonomics remain later work.

Docs: [README](README.md), [aiguide/GUIDE.md](aiguide/GUIDE.md), [aiguide/CURSOR_CODEX_MCP_GUIDE.md](aiguide/CURSOR_CODEX_MCP_GUIDE.md).

---

## v1.2.0 — Wipe, seed, cache registry, Routes() (2026-08-09)

Historical tag. Prefer **v1.2.1** for new installs. Feature notes below describe what first shipped with this line.

Vormia Go **v1.2.0** unlocks the library surface for future CLI Group C commands (`db:wipe`, `migrate:fresh`, `db:seed`, `db:fresh`, `cache:ping` / `cache:forget`, `route:list`). The CLI wrappers themselves are **not** in this module.

**Pair with** [vormia-go-driver-chi](https://github.com/vormialabs/vormia-go-driver-chi) **v1.1.0+** — older chi builds do not implement `Routes()`.

### What this release includes

**`db.Wipe(ctx, conn, driver)`** — drops every user table on a connection. Dialect-specific enumeration (`sqlite_master` / `pg_tables` / `information_schema`). SQLite and Postgres use a transaction; MySQL toggles `FOREIGN_KEY_CHECKS` and is **non-atomic**.

**`seed.Run(ctx, conn, src)`** — executes every `*.sql` file in an `fs.FS` in filename order. No tracking table; seeders should be idempotent.

**`cache.Registry`** — named cache connections via `CACHE_*` / `CACHE_<NAME>_*` and `CACHE_CONNECTION`, same opener pattern as `db.Registry`. Does **not** add `Flush` to `contract.Cache` (so a true `cache:clear` still waits on a future contract change).

**`Router.Routes() []RouteInfo`** — the one contract change in this release. `RouteInfo` is a type alias to an anonymous struct so drivers can satisfy it without importing vormia-go.

Also includes everything from v1.1.3 (registry, migrator including `Status`, kernel, contracts).

### Install

```bash
go get github.com/vormialabs/vormia-go@v1.2.0
go get github.com/vormialabs/vormia-go-driver-chi@v1.1.0
```

Requires Go 1.26+. Prefer **v1.2.1** for new installs.

### Testing

```bash
go test -v ./...
```

Boundary check: `go list -deps ./db ./migrate ./seed ./cache` must not list any `vormia-go-driver-*` module.

### What's not in this release

CLI Group C command wrappers (`db:wipe`, `migrate:fresh`, …) live in vormia-go-cli (separate plan). `Cache.Flush` / `cache:clear`, ORM, validation, auth, and HTTP ergonomics remain later work.

Docs: [README](README.md), [aiguide/GUIDE.md](aiguide/GUIDE.md), [aiguide/CURSOR_CODEX_MCP_GUIDE.md](aiguide/CURSOR_CODEX_MCP_GUIDE.md).

---

## v1.1.3 — Migrator.Status (2026-08-07)

Historical tag. Prefer **v1.2.1** for new installs. Feature notes below describe what first shipped with this line.

Vormia Go **v1.1.3** includes everything from the connection registry and migration engine, plus `Migrator.Status`.

Do not use `v1.1.1` or `v1.1.2`.

### What this release includes

**`Status(ctx)`** returns `[]StatusRow` for every `<version>.up.sql` found in the migration source, in apply order:

| Field | Meaning |
|-------|---------|
| `Version` | Migration version (filename without `.up.sql`) |
| `Applied` | `true` if the version is in `schema_migrations` |
| `Batch` | Batch number when applied; `0` if pending |

Ensures the tracking table exists (same as `Up` / `Version`), then joins disk versions with applied rows. No SQL files are executed.

This is the library surface a thin `migrate:status` CLI will call; the CLI itself is still not in this module.

Also includes: named connection registry (`db`), migration engine (`migrate`: `Up`, `Rollback`, `Reset`, `Version`), kernel, and contracts.

### Install

```bash
go get github.com/vormialabs/vormia-go@v1.1.3
```

Requires Go 1.26+. Prefer **v1.2.1** for new installs.

### Testing

```bash
go test -v ./...
```

### What's not in this release

CLI wrappers and `db.Wipe` / `seed` / `cache.Registry` / `Routes()` shipped later in **v1.2.0**. ORM, validation, auth remain later work.

Docs: [README](README.md), [aiguide/GUIDE.md](aiguide/GUIDE.md), [aiguide/CURSOR_CODEX_MCP_GUIDE.md](aiguide/CURSOR_CODEX_MCP_GUIDE.md).

---

## v1.1.1 — Migrator.Status (2026-08-07)

Historical tag. Prefer **v1.2.1** for new installs. Feature notes below describe what first shipped with this line.

**`Status(ctx)`** returns `[]StatusRow` for every `<version>.up.sql` found in the migration source, in apply order (`Version`, `Applied`, `Batch`). Ensures the tracking table exists; does not execute migration SQL. Intended as the library surface for a future `migrate:status` CLI.

---

## v1.1.0 — Connection Registry + Migration Engine (2026-08-07)

Vormia Go **v1.1.0** adds two engine-agnostic packages on top of the Slice 1 kernel: a named connection registry and a SQL migration engine. Both sit on `contract.Database` and import no concrete drivers.

### What this release does

**Connection registry (`db`).** Resolve connections from the config convention locked with vormia-go-core:

- `default` → bare `DB_*` keys (`DB_DRIVER`, `DB_HOST`, `DB_PATH`, …)
- Named connection `mysql2` → `DB_MYSQL2_*`
- `DB_CONNECTION` selects which name is default (fallback `"default"`)

The app registers one `Opener` per driver it imports; the registry looks up the opener by `DRIVER` and caches live connections. Missing opener / missing group / missing `DRIVER` errors name the exact fix. The framework still never imports `driver-postgresql`, `driver-mysql`, or `driver-sqlite`.

**Migration engine (`migrate`).** Apply and roll back `<version>.up.sql` / `<version>.down.sql` files from any `fs.FS` (e.g. `os.DirFS("database/migrations")`). Tracks applied versions in a portable `schema_migrations` table (`version`, `batch`, `applied_at`). Supports:

- `Up` — all pending migrations in one new batch
- `Rollback(steps)` — `steps <= 0` rolls back the latest batch; otherwise that many, newest first
- `Reset` — roll everything back
- `Version` — latest applied version

(`Status` for applied vs pending ships in **v1.1.3**; first attempted in **v1.1.1**.)

Placeholder style for tracking-table inserts/deletes goes through the driver's `Rebind`.

**Dependency.** Production `db` depends on `vormia-go-core@v1.1.0` (`config.GetString`, `config.Prefixed`). Production `migrate` depends only on `contract` + stdlib. Drivers remain test-only (SQLite in-memory + `fstest.MapFS`).

### Packages now in the module

| Package | Purpose |
|---------|---------|
| `contract` | `Router`, `Database`, `Cache` |
| `app` | Kernel |
| `db` | Named connection registry |
| `migrate` | SQL migrations |

### Install

```bash
go get github.com/vormialabs/vormia-go@v1.1.0
```

Requires Go 1.26+. Prefer **v1.2.1** for new installs.

### Testing

```bash
go test -v ./...
```

Boundary check: `go list -deps ./db ./migrate` must not list any `vormia-go-driver-*` module.

### What's not in this release

CLI `migrate*` / `db:*` commands, `--database` flag wiring, and starterkit opener scaffolding land in the next step. `Migrator.Status` ships in **v1.1.3**. ORM, validation, auth, and HTTP ergonomics remain later work.

Docs: [README](README.md), [aiguide/GUIDE.md](aiguide/GUIDE.md), [aiguide/CURSOR_CODEX_MCP_GUIDE.md](aiguide/CURSOR_CODEX_MCP_GUIDE.md).

---

## v1.0.1 — Official Slice 1 Release (2026-08-04)

Vormia Go **v1.0.1** is the official Slice 1 release. It delivers the framework's spine: you can boot an application, wire in your chosen drivers, register routes and middleware, serve HTTP, and shut down gracefully — all without the framework ever depending on a concrete router, database, or cache implementation.

This tag supersedes `v1.0.0` as the Slice 1 install target of its day. Prefer **v1.2.1** for new installs.

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

ORM/query builder, validation, auth, and CLI scaffolding are planned for later slices. Connection registry and migrations shipped later in **v1.1.0**; `Migrator.Status` in **v1.1.3**.

---

## v1.0.0 — Slice 1: Kernel + Contracts (2026-07-28)

Initial Slice 1 tag. Prefer **v1.2.1** for new installs.
