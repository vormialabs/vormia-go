// Package cache resolves named cache connections from config and opens them
// through app-registered driver openers. It imports no concrete driver.
package cache

import (
	"fmt"
	"strings"
	"sync"

	"github.com/vormialabs/vormia-go-core/config"
	"github.com/vormialabs/vormia-go/contract"
)

// ConnConfig is the resolved settings for one named cache connection.
type ConnConfig struct {
	Name   string
	Driver string // e.g. "redis"
	Addr   string
	Extra  map[string]string
}

// Opener builds a live cache from a resolved config. The APP registers
// one per driver it imports — the framework never imports a cache driver.
type Opener func(ConnConfig) (contract.Cache, error)

// Registry holds openers, resolves connection configs, and caches live
// connections by name.
type Registry struct {
	cfg     *config.Config
	mu      sync.Mutex
	openers map[string]Opener
	live    map[string]contract.Cache
}

// New creates a registry bound to a config source.
func New(cfg *config.Config) *Registry {
	return &Registry{
		cfg:     cfg,
		openers: map[string]Opener{},
		live:    map[string]contract.Cache{},
	}
}

// RegisterOpener wires a driver name to its opener.
func (r *Registry) RegisterOpener(driver string, o Opener) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.openers[driver] = o
}

// Default returns the connection name used when none is specified.
func (r *Registry) Default() string {
	return r.cfg.GetString("CACHE_CONNECTION", "default")
}

// Resolve builds a ConnConfig from config for the given name.
// The "default" connection reads bare CACHE_* keys; any other name reads
// CACHE_<NAME>_* keys.
func (r *Registry) Resolve(name string) (ConnConfig, error) {
	if name == "" {
		name = r.Default()
	}

	if name == "default" {
		return ConnConfig{
			Name:   "default",
			Driver: r.cfg.GetString("CACHE_DRIVER", ""),
			Addr:   r.cfg.GetString("CACHE_ADDR", ""),
		}, nil
	}

	group := r.cfg.Prefixed("CACHE_" + strings.ToUpper(name) + "_")
	if len(group) == 0 {
		return ConnConfig{}, fmt.Errorf(
			"no configuration for cache connection %q (expected CACHE_%s_* keys)",
			name, strings.ToUpper(name))
	}
	return ConnConfig{
		Name:   name,
		Driver: group["DRIVER"],
		Addr:   group["ADDR"],
		Extra:  group,
	}, nil
}

// Connection resolves, opens (once, cached), and returns a live cache.
// An empty name means the default connection.
func (r *Registry) Connection(name string) (contract.Cache, error) {
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
		return nil, fmt.Errorf("cache connection %q has no DRIVER set", name)
	}

	opener, ok := r.openers[cc.Driver]
	if !ok {
		return nil, fmt.Errorf(
			"no opener registered for cache driver %q (connection %q) — the app must import that driver and call RegisterOpener(%q, ...)",
			cc.Driver, name, cc.Driver)
	}

	c, err := opener(cc)
	if err != nil {
		return nil, fmt.Errorf("open cache connection %q: %w", name, err)
	}
	r.live[name] = c
	return c, nil
}

// Close closes every live cache connection.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var firstErr error
	for name, c := range r.live {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close %q: %w", name, err)
		}
		delete(r.live, name)
	}
	return firstErr
}
