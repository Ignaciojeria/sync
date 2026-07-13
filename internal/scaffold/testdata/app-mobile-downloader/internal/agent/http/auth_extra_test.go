package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type stringerImpl struct{ s string }

func (s stringerImpl) String() string { return s.s }

func TestAnyString(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"hello", "hello"},
		{stringerImpl{s: "from-stringer"}, "from-stringer"},
		{42, "42"},
		{true, "true"},
	}
	for _, c := range cases {
		if got := anyString(c.in); got != c.want {
			t.Errorf("anyString(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("under = %q", got)
	}
	if got := truncate("  hello  ", 10); got != "hello" {
		t.Errorf("trim under = %q", got)
	}
	if got := truncate("hello world", 5); got != "hello…" {
		t.Errorf("truncated = %q", got)
	}
	if got := truncate("hello", 0); got != "" {
		t.Errorf("n=0 = %q", got)
	}
	if got := truncate("hello", -1); got != "" {
		t.Errorf("negative = %q", got)
	}
}

func TestJWTSummaryEmpty(t *testing.T) {
	if got := jwtSummary(""); got != "(empty)" {
		t.Errorf("empty = %q", got)
	}
}

func TestJWTSummaryInvalidFormat(t *testing.T) {
	if got := jwtSummary("not-a-jwt"); !strings.HasPrefix(got, "(no es un JWT") {
		t.Errorf("invalid = %q", got)
	}
}

func TestSessionIDFromCookieEmpty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "app_session_id", Value: "   "})
	if _, ok := sessionIDFromCookie(req); ok {
		t.Error("cookie vacío debería devolver ok=false")
	}
}
