// Package db resolves named database connections from config and opens them
// through app-registered driver openers. It imports no concrete driver.
package db

import (
	"fmt"
	"strings"
	"sync"

	"github.com/vormialabs/vormia-go-core/config"
	"github.com/vormialabs/vormia-go/contract"
)

// ConnConfig is the resolved settings for one named connection.
type ConnConfig struct {
	Name     string
	Driver   string // "sqlite" | "postgres" | "mysql"
	Host     string
	Port     string
	User     string
	Password string
	Database string
	SSLMode  string
	Path     string            // sqlite
	Extra    map[string]string // any other keys in the group
}

// Opener builds a live connection from a resolved config. The APP registers
// one per driver it imports — this is how the framework opens a driver it
// never imports.
type Opener func(ConnConfig) (contract.Database, error)

// Registry holds openers, resolves connection configs, and caches live
// connections by name.
type Registry struct {
	cfg     *config.Config
	mu      sync.Mutex
	openers map[string]Opener
	live    map[string]contract.Database
}

// New creates a registry bound to a config source.
func New(cfg *config.Config) *Registry {
	return &Registry{
		cfg:     cfg,
		openers: map[string]Opener{},
		live:    map[string]contract.Database{},
	}
}

// RegisterOpener wires a driver name to its opener. Called by the app once
// per driver it imports.
func (r *Registry) RegisterOpener(driver string, o Opener) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.openers[driver] = o
}

// Default returns the connection name used when none is specified.
func (r *Registry) Default() string {
	return r.cfg.GetString("DB_CONNECTION", "default")
}

// Resolve builds a ConnConfig from config for the given name.
// The "default" connection reads bare DB_* keys; any other name reads
// DB_<NAME>_* keys (via config.Prefixed from step 1).
func (r *Registry) Resolve(name string) (ConnConfig, error) {
	if name == "" {
		name = r.Default()
	}

	if name == "default" {
		return ConnConfig{
			Name:     "default",
			Driver:   r.cfg.GetString("DB_DRIVER", ""),
			Host:     r.cfg.GetString("DB_HOST", ""),
			Port:     r.cfg.GetString("DB_PORT", ""),
			User:     r.cfg.GetString("DB_USER", ""),
			Password: r.cfg.GetString("DB_PASSWORD", ""),
			Database: r.cfg.GetString("DB_NAME", ""),
			SSLMode:  r.cfg.GetString("DB_SSLMODE", ""),
			Path:     r.cfg.GetString("DB_PATH", ""),
		}, nil
	}

	group := r.cfg.Prefixed("DB_" + strings.ToUpper(name) + "_")
	if len(group) == 0 {
		return ConnConfig{}, fmt.Errorf(
			"no configuration for connection %q (expected DB_%s_* keys)",
			name, strings.ToUpper(name))
	}
	cc := ConnConfig{
		Name:     name,
		Driver:   group["DRIVER"],
		Host:     group["HOST"],
		Port:     group["PORT"],
		User:     group["USER"],
		Password: group["PASSWORD"],
		Database: group["NAME"],
		SSLMode:  group["SSLMODE"],
		Path:     group["PATH"],
		Extra:    group,
	}
	return cc, nil
}

// Connection resolves, opens (once, cached), and returns a live connection.
// An empty name means the default connection.
func (r *Registry) Connection(name string) (contract.Database, error) {
	if name == "" {
		name = r.Default()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if live, ok := r.live[name]; ok {
		return live, nil
	}

	cc, err := r.Resolve(name)
	if err != nil {
		return nil, err
	}
	if cc.Driver == "" {
		return nil, fmt.Errorf("connection %q has no DRIVER set", name)
	}

	opener, ok := r.openers[cc.Driver]
	if !ok {
		return nil, fmt.Errorf(
			"no opener registered for driver %q (connection %q) — the app must import that driver and call RegisterOpener(%q, ...)",
			cc.Driver, name, cc.Driver)
	}

	conn, err := opener(cc)
	if err != nil {
		return nil, fmt.Errorf("open connection %q: %w", name, err)
	}
	r.live[name] = conn
	return conn, nil
}

// Close closes every live connection.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var firstErr error
	for name, conn := range r.live {
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close %q: %w", name, err)
		}
		delete(r.live, name)
	}
	return firstErr
}
