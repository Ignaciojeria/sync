package layout

import (
	"net/http"
	"net/http/httptest"
	"testing"

	authmiddleware "fixtests1/internal/auth/middleware"
	"fixtests1/internal/shared/configuration"
)

// TestFromRequestWithClaimsUsesEditorEmail wires the JWT middleware in
// AUTH_DISABLED mode so a real-looking claims value is injected into the
// request context. FromRequest then exercises its post-claims branch.
func TestFromRequestWithClaimsUsesEditorEmail(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")

	mw := authmiddleware.JWTMiddleware(nil, nil, configuration.Conf{})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nav := FromRequest(r)
		if !nav.IsEditor {
			t.Fatalf("expected IsEditor=true for editor email, got %+v", nav)
		}
		if nav.CurrentPath != "/report/tests" {
			t.Fatalf("CurrentPath = %q", nav.CurrentPath)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/report/tests", nil)
	req.Header.Set("X-Dev-Email", "dev@example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

func TestFromRequestWithNonEditorClaims(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")

	mw := authmiddleware.JWTMiddleware(nil, nil, configuration.Conf{})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nav := FromRequest(r)
		if nav.IsEditor {
			t.Fatalf("expected IsEditor=false for unknown email, got %+v", nav)
		}
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Dev-Email", "stranger@example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

func TestFromRequestWithEmptyEmailClaims(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")

	mw := authmiddleware.JWTMiddleware(nil, nil, configuration.Conf{})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nav := FromRequest(r)
		// X-Dev-Email is empty → middleware defaults to dev@example.com which
		// IS in the editor allowlist. Confirm FromRequest follows the same
		// membership policy the middleware applies.
		if !nav.IsEditor {
			t.Fatalf("expected IsEditor=true for dev default email, got %+v", nav)
		}
	}))

	req := httptest.NewRequest("GET", "/", nil)
	// Whitespace-only X-Dev-Email — middleware trims and falls back to dev default.
	req.Header.Set("X-Dev-Email", "  ")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}
