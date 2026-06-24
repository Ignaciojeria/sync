package shared

import (
	"context"
	"errors"
	"testing"
)

func TestHooksRegisterShutdownInvokesAllHooksInOrder(t *testing.T) {
	var order []int
	h := &Hooks{}
	h.RegisterShutdown(func() error { order = append(order, 1); return nil })
	h.RegisterShutdown(func() error { order = append(order, 2); return nil })
	h.RegisterShutdown(func() error { order = append(order, 3); return nil })

	if err := h.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("hook order = %v, want [1 2 3]", order)
	}
}

func TestHooksShutdownAggregatesErrors(t *testing.T) {
	h := &Hooks{}
	h.RegisterShutdown(func() error { return errors.New("first failure") })
	h.RegisterShutdown(func() error { return nil })
	h.RegisterShutdown(func() error { return errors.New("third failure") })

	err := h.Shutdown(context.Background())
	if err == nil {
		t.Fatal("expected aggregated error")
	}

	msg := err.Error()
	if !contains(msg, "first failure") || !contains(msg, "third failure") || contains(msg, "second") {
		t.Fatalf("error message = %q, expected both failures", msg)
	}
}

func TestHooksShutdownNoHooksIsNoop(t *testing.T) {
	if err := (&Hooks{}).Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v, want nil", err)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		(len(needle) > 0 && indexOf(haystack, needle) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
