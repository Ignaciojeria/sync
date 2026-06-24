package layout

import (
	"net/http/httptest"
	"testing"
)

func TestFromRequestWithoutClaimsReturnsEditorFalse(t *testing.T) {
	req := httptest.NewRequest("GET", "/some-path", nil)
	nav := FromRequest(req)
	if nav.CurrentPath != "/some-path" {
		t.Fatalf("CurrentPath = %q", nav.CurrentPath)
	}
	if nav.IsEditor {
		t.Fatal("expected IsEditor=false without claims")
	}
}

func TestNavigationContextStructFields(t *testing.T) {
	nav := NavigationContext{CurrentPath: "/x", IsEditor: true}
	if nav.CurrentPath != "/x" || !nav.IsEditor {
		t.Fatal("expected NavigationContext fields to round-trip")
	}
}
