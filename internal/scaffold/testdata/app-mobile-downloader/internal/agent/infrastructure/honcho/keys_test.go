package honcho

import (
	"strings"
	"testing"
)

func TestUserPeerIDFor_StableAcrossCase(t *testing.T) {
	t.Parallel()
	// El peer ID debe ser estable independiente de mayúsculas y
	// espacios, porque un usuario con email "Alice@Example.com"
	// y "alice@example.com" es el mismo humano.
	id1 := userPeerIDFor("Alice@Example.com")
	id2 := userPeerIDFor("  alice@example.com  ")
	if id1 != id2 {
		t.Errorf("userPeerIDFor case-insensitive: %q != %q", id1, id2)
	}
}

func TestUserPeerIDFor_DifferentForDifferentEmails(t *testing.T) {
	t.Parallel()
	// Dos emails distintos deben dar peer IDs distintos (con
	// probabilidad negligible de colisión a 64 bits).
	id1 := userPeerIDFor("alice@example.com")
	id2 := userPeerIDFor("bob@example.com")
	if id1 == id2 {
		t.Errorf("userPeerIDFor collision: %q == %q", id1, id2)
	}
}

func TestUserPeerIDFor_PrivacyNoLeakEmail(t *testing.T) {
	t.Parallel()
	// El peer ID no debe contener el email en claro.
	id := userPeerIDFor("alice@example.com")
	if strings.Contains(id, "alice") || strings.Contains(id, "@") || strings.Contains(id, ".") {
		t.Errorf("userPeerIDFor leaks email: %q", id)
	}
}

func TestUserPeerIDFor_HonchoPattern(t *testing.T) {
	t.Parallel()
	// El ID generado debe matchear la regex de Honcho. Si no, el
	// adapter va a tirar 422 silencioso en producción.
	for _, email := range []string{
		"alice@example.com",
		"very-long.email+tag@subdomain.example.com",
		"用户@example.com",
		"a@b.c",
	} {
		id := userPeerIDFor(email)
		if !matchesHonchoPattern(id) {
			t.Errorf("userPeerIDFor(%q) = %q, does not match Honcho pattern", email, id)
		}
	}
}

func TestMatchesHonchoPattern(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want bool
	}{
		{"agent-1784425654933158393-1", true},
		{"user-abc123", true},
		{"a", true},
		{"ABC_def-123", true},
		{"", false},
		{"user:alice@example.com", false}, // colon y @ no permitidos
		{"user-alice@example.com", false}, // @ no permitido
		{"user.alice", false},             // . no permitido
		{"user alice", false},             // espacio no permitido
		{"user/alice", false},             // / no permitido
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := matchesHonchoPattern(tt.in); got != tt.want {
				t.Errorf("matchesHonchoPattern(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveIDs_RejectsBadSessionID(t *testing.T) {
	t.Parallel()
	// Si SessionID tiene chars que Honcho rechazaría, fallamos
	// explícito en vez de mandar basura.
	_, err := resolveIDs(sampleKey())
	if err != nil {
		t.Fatalf("sampleKey() should be valid: %v", err)
	}
	bad := sampleKey()
	bad.SessionID = "has:colon"
	if _, err := resolveIDs(bad); err == nil {
		t.Error("expected error for SessionID with colon, got nil")
	}
}
