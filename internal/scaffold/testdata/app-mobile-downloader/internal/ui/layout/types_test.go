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

func TestFromRequestStripsPreviewPrefixFromCurrentPath(t *testing.T) {
	req := httptest.NewRequest("GET", "/agent/sessions/s1/preview/scheduler/jobs", nil)
	req.Header.Set("X-Forwarded-Prefix", "/agent/sessions/s1/preview/")
	nav := FromRequest(req)
	if got, want := nav.PreviewPrefix, "/agent/sessions/s1/preview/"; got != want {
		t.Fatalf("PreviewPrefix = %q, want %q", got, want)
	}
	if got, want := nav.CurrentPath, "/scheduler/jobs"; got != want {
		t.Fatalf("CurrentPath = %q, want %q", got, want)
	}
}

func TestNavigationContextAppPathKeepsAgentUIOutsidePreview(t *testing.T) {
	nav := NavigationContext{PreviewPrefix: "/agent/sessions/s1/preview/"}
	if got, want := nav.AppPath("/scheduler/jobs"), "/agent/sessions/s1/preview/scheduler/jobs"; got != want {
		t.Fatalf("AppPath = %q, want %q", got, want)
	}
	if got, want := nav.HostPath("/agent"), "/agent"; got != want {
		t.Fatalf("HostPath = %q, want %q", got, want)
	}
}
