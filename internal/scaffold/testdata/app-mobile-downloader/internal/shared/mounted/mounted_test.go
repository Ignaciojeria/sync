package mounted

import (
	"net/http"
	"testing"
)

func TestReadReturnToRejectsUnsafeValues(t *testing.T) {
	req := &http.Request{Header: make(http.Header)}
	req.AddCookie(&http.Cookie{Name: ReturnToCookieName, Value: "https://evil.example"})
	if got := ReadReturnTo(req); got != "" {
		t.Fatalf("ReadReturnTo() = %q, want empty", got)
	}
}

func TestReadReturnToAcceptsLocalMountedPath(t *testing.T) {
	req := &http.Request{Header: make(http.Header)}
	req.AddCookie(&http.Cookie{Name: ReturnToCookieName, Value: "/agent/sessions/s-1/preview/report/tests?tab=1"})
	if got := ReadReturnTo(req); got != "/agent/sessions/s-1/preview/report/tests?tab=1" {
		t.Fatalf("ReadReturnTo() = %q", got)
	}
}

func TestPrefixFallsBackToPreviewPath(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/agent/sessions/s-1/preview/auth/login", nil)
	if got := Prefix(req); got != "/agent/sessions/s-1/preview/" {
		t.Fatalf("Prefix() = %q", got)
	}
}
