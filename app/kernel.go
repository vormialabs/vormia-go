package app

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vormialabs/vormia-go/contract"
)

// Kernel is the running application. It holds the router and, optionally,
// a database and cache — all as interfaces, so it never depends on a
// concrete driver.
type Kernel struct {
	Router contract.Router
	DB     contract.Database // may be nil
	Cache  contract.Cache    // may be nil
}

// Option configures a Kernel at construction time.
type Option func(*Kernel)

// WithDB attaches a database driver.
func WithDB(db contract.Database) Option { return func(k *Kernel) { k.DB = db } }

// WithCache attaches a cache driver.
func WithCache(c contract.Cache) Option { return func(k *Kernel) { k.Cache = c } }

// New builds a Kernel around a router, plus any optional drivers.
func New(router contract.Router, opts ...Option) *Kernel {
	k := &Kernel{Router: router}
	for _, o := range opts {
		o(k)
	}
	return k
}

// Use registers global middleware. Call before Routes.
func (k *Kernel) Use(mw ...func(http.Handler) http.Handler) {
	k.Router.Use(mw...)
}

// Routes hands the router to the caller to register routes.
func (k *Kernel) Routes(register func(r contract.Router)) {
	register(k.Router)
}

// Run starts the server and blocks until SIGINT/SIGTERM, then shuts down
// gracefully and closes attached drivers.
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

func (k *Kernel) closeDrivers() {
	if k.DB != nil {
		_ = k.DB.Close()
	}
	if k.Cache != nil {
		_ = k.Cache.Close()
	}
}
