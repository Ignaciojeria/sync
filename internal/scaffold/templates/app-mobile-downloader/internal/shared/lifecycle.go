package shared

import (
	"context"
	"errors"
	"log"
)

// Hooks is a tiny shutdown-hook registry so wiring stays explicit in main()
// instead of bouncing through a DI container.
type Hooks struct {
	hooks []func() error
}

func (h *Hooks) RegisterShutdown(f func() error) {
	h.hooks = append(h.hooks, f)
}

func (h *Hooks) Shutdown(ctx context.Context) error {
	var errs []error
	for _, fn := range h.hooks {
		if err := fn(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		log.Printf("shutdown completed with errors: %v", err)
		return err
	}
	return nil
}
