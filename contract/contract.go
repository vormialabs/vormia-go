// Package contract declares the interfaces that Vormia drivers satisfy.
// It imports only the standard library, so drivers can satisfy these
// structurally without importing the framework.
package contract

import (
	"context"
	"database/sql"
	"net/http"
	"time"
)

// RouteInfo is one registered route (method + pattern). Defined as a type
// alias to an anonymous struct so router drivers can return the identical
// type without importing this package (avoids a chi ↔ vormia-go module cycle).
type RouteInfo = struct {
	Method  string
	Pattern string
}

// Router is satisfied by router drivers (chi today; gin/echo later).
// Note: Group/Route are intentionally absent — they are concrete-typed on
// each driver. Only the stdlib-shaped surface is portable.
type Router interface {
	Get(pattern string, h http.HandlerFunc)
	Post(pattern string, h http.HandlerFunc)
	Put(pattern string, h http.HandlerFunc)
	Patch(pattern string, h http.HandlerFunc)
	Delete(pattern string, h http.HandlerFunc)
	Head(pattern string, h http.HandlerFunc)
	Options(pattern string, h http.HandlerFunc)
	Use(mw ...func(http.Handler) http.Handler)
	Serve(addr string) error
	Shutdown(ctx context.Context) error
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	Routes() []RouteInfo
}

// Database is satisfied by SQL drivers (sqlite, postgres, mysql). Every
// method except Rebind comes free from an embedded *sql.DB.
type Database interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	PingContext(ctx context.Context) error
	Close() error
	Rebind(query string) string
}

// Cache is satisfied by cache drivers (redis today; memory/file later).
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Ping(ctx context.Context) error
	Close() error
}
