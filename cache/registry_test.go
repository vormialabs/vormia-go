package cache_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vormialabs/vormia-go-core/config"
	"github.com/vormialabs/vormia-go/cache"
	"github.com/vormialabs/vormia-go/contract"
)

// stubCache is a minimal in-memory contract.Cache for registry tests.
type stubCache struct {
	store map[string][]byte
}

func newStub() *stubCache {
	return &stubCache{store: map[string][]byte{}}
}

func (s *stubCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	v, ok := s.store[key]
	return v, ok, nil
}
func (s *stubCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	s.store[key] = value
	return nil
}
func (s *stubCache) Delete(_ context.Context, key string) error {
	delete(s.store, key)
	return nil
}
func (s *stubCache) Exists(_ context.Context, key string) (bool, error) {
	_, ok := s.store[key]
	return ok, nil
}
func (s *stubCache) Ping(context.Context) error { return nil }
func (s *stubCache) Close() error                { return nil }

func TestResolveDefaultAndNamed(t *testing.T) {
	cfg := config.New()
	cfg.Set("CACHE_DRIVER", "memory")
	cfg.Set("CACHE_ADDR", "localhost:6379")
	cfg.Set("CACHE_REDIS2_DRIVER", "memory")
	cfg.Set("CACHE_REDIS2_ADDR", "127.0.0.1:6380")

	reg := cache.New(cfg)

	def, err := reg.Resolve("default")
	if err != nil {
		t.Fatalf("resolve default: %v", err)
	}
	if def.Driver != "memory" || def.Addr != "localhost:6379" {
		t.Fatalf("default config: %+v", def)
	}

	named, err := reg.Resolve("redis2")
	if err != nil {
		t.Fatalf("resolve redis2: %v", err)
	}
	if named.Driver != "memory" || named.Addr != "127.0.0.1:6380" {
		t.Fatalf("named config: %+v", named)
	}
}

func TestConnectionOpenerDispatchAndCache(t *testing.T) {
	cfg := config.New()
	cfg.Set("CACHE_CONNECTION", "default")
	cfg.Set("CACHE_DRIVER", "memory")
	cfg.Set("CACHE_ADDR", "local")
	cfg.Set("CACHE_OTHER_DRIVER", "memory")
	cfg.Set("CACHE_OTHER_ADDR", "other")

	reg := cache.New(cfg)

	var seen []cache.ConnConfig
	reg.RegisterOpener("memory", func(c cache.ConnConfig) (contract.Cache, error) {
		seen = append(seen, c)
		return newStub(), nil
	})

	c1, err := reg.Connection("")
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	if len(seen) != 1 || seen[0].Name != "default" {
		t.Fatalf("opener args: %+v", seen)
	}

	c2, err := reg.Connection("")
	if err != nil {
		t.Fatalf("cached: %v", err)
	}
	if c1 != c2 {
		t.Fatal("expected Connection to cache the live default")
	}
	if len(seen) != 1 {
		t.Fatalf("opener should not run again, calls=%d", len(seen))
	}

	if _, err := reg.Connection("other"); err != nil {
		t.Fatalf("named: %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("expected second opener call, calls=%d", len(seen))
	}

	if err := reg.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestConnectionErrors(t *testing.T) {
	cfg := config.New()
	cfg.Set("CACHE_DRIVER", "memory")
	reg := cache.New(cfg)

	if _, err := reg.Resolve("missing"); err == nil || !strings.Contains(err.Error(), "CACHE_MISSING_*") {
		t.Fatalf("expected missing-group error, got %v", err)
	}

	if _, err := reg.Connection("default"); err == nil || !strings.Contains(err.Error(), "no opener registered") {
		t.Fatalf("expected missing-opener error, got %v", err)
	}

	cfg2 := config.New()
	cfg2.Set("CACHE_ADDR", "localhost")
	reg2 := cache.New(cfg2)
	reg2.RegisterOpener("memory", func(c cache.ConnConfig) (contract.Cache, error) {
		return newStub(), nil
	})
	if _, err := reg2.Connection("default"); err == nil || !strings.Contains(err.Error(), "no DRIVER set") {
		t.Fatalf("expected missing-driver error, got %v", err)
	}
}
