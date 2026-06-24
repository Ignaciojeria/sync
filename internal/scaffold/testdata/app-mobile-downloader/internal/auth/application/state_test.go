package auth

import (
	"encoding/base64"
	"testing"
)

func TestNewRandomState(t *testing.T) {
	s, err := NewRandomState()
	if err != nil {
		t.Fatalf("NewRandomState() error = %v", err)
	}
	if s == "" {
		t.Fatal("expected non-empty state")
	}

	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("state is not valid base64url: %v", err)
	}
	if len(raw) != 32 {
		t.Fatalf("decoded length = %d, want 32", len(raw))
	}

	// Two consecutive calls must return different values.
	s2, err := NewRandomState()
	if err != nil {
		t.Fatalf("NewRandomState() #2 error = %v", err)
	}
	if s == s2 {
		t.Fatal("expected different state values across calls")
	}
}
