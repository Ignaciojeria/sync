package mounted

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "/"},
		{"/", "/"},
		{"foo", "/foo"},
		{"/foo", "/foo"},
		{"  /foo/bar  ", "/foo/bar"},
	}
	for _, c := range cases {
		if got := NormalizePath(c.in); got != c.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAppJoinsPrefixAndPath(t *testing.T) {
	if got := App("", "/foo"); got != "/foo" {
		t.Errorf("App(\"\", \"/foo\") = %q, want /foo", got)
	}
	if got := App("/agent", "/"); got != "/agent/" {
		t.Errorf("App with root = %q, want /agent/", got)
	}
	if got := App("/agent", "/sessions/1"); got != "/agent/sessions/1" {
		t.Errorf("App with subpath = %q", got)
	}
	if got := App("/agent/", "/sessions/1"); got != "/agent/sessions/1" {
		t.Errorf("App trims trailing slash = %q", got)
	}
	if got := App("  ", "/x"); got != "/x" {
		t.Errorf("App empty prefix = %q", got)
	}
}

func TestHostNormalizes(t *testing.T) {
	if got := Host("  /foo  "); got != "/foo" {
		t.Errorf("Host() = %q", got)
	}
}

func TestRelativeStripsPrefix(t *testing.T) {
	if got := Relative("", "/foo"); got != "/foo" {
		t.Errorf("Relative no prefix = %q", got)
	}
	if got := Relative("/agent", "/agent"); got != "/" {
		t.Errorf("Relative exact match = %q, want /", got)
	}
	if got := Relative("/agent", "/agent/sessions/1"); got != "/sessions/1" {
		t.Errorf("Relative subpath = %q", got)
	}
	if got := Relative("/agent/", "/other"); got != "/other" {
		t.Errorf("Relative unrelated = %q", got)
	}
}

func TestCurrentAppURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/agent/sessions/s-1/preview/foo?x=1", nil)
	req.Header.Set("X-Forwarded-Prefix", "/agent/sessions/s-1/preview")
	if got := CurrentAppURL(req); got != "/agent/sessions/s-1/preview/foo?x=1" {
		t.Errorf("CurrentAppURL() = %q", got)
	}
}

func TestCurrentAppURLNil(t *testing.T) {
	if got := CurrentAppURL(nil); got != "/" {
		t.Errorf("CurrentAppURL(nil) = %q, want /", got)
	}
}

func TestSetReturnToCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	SetReturnToCookie(rec, nil, "/foo", true)
	cookie := rec.Result().Cookies()
	if len(cookie) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookie))
	}
	if cookie[0].Name != ReturnToCookieName || cookie[0].Value != "/foo" {
		t.Errorf("cookie = %+v", cookie[0])
	}
	if !cookie[0].Secure || !cookie[0].HttpOnly {
		t.Errorf("cookie flags wrong: %+v", cookie[0])
	}
}

func TestSetReturnToCookieDefaultsToCurrentURL(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/agent/foo?z=2", nil)
	req.Header.Set("X-Forwarded-Prefix", "/agent")
	SetReturnToCookie(rec, req, "  ", true)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value != "/agent/foo?z=2" {
		t.Errorf("default returnTo = %+v", cookies)
	}
}

func TestSetReturnToCookieNilWriterIsNoOp(t *testing.T) {
	SetReturnToCookie(nil, nil, "/x", false) // debe no panic
}

func TestClearReturnToCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	ClearReturnToCookie(rec, true)
	cookie := rec.Result().Cookies()
	if len(cookie) != 1 {
		t.Fatalf("got %d cookies", len(cookie))
	}
	if cookie[0].MaxAge != -1 || cookie[0].Value != "" {
		t.Errorf("clear cookie = %+v", cookie[0])
	}
}

func TestClearReturnToCookieNilWriterIsNoOp(t *testing.T) {
	ClearReturnToCookie(nil, false)
}

func TestIsSafeReturnTo(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"  ", false},
		{"https://evil.example", false},
		{"//evil.example", false},
		{"/safe/path", true},
		{"/safe?x=1", true},
	}
	for _, c := range cases {
		if got := IsSafeReturnTo(c.in); got != c.want {
			t.Errorf("IsSafeReturnTo(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestPrefixNilRequest(t *testing.T) {
	if got := Prefix(nil); got != "" {
		t.Errorf("Prefix(nil) = %q, want empty", got)
	}
}

func TestReadReturnToMissingCookie(t *testing.T) {
	req := &http.Request{Header: make(http.Header)}
	if got := ReadReturnTo(req); got != "" {
		t.Errorf("ReadReturnTo sin cookie = %q", got)
	}
}

func TestReadReturnToNilRequest(t *testing.T) {
	if got := ReadReturnTo(nil); got != "" {
		t.Errorf("ReadReturnTo(nil) = %q", got)
	}
}

func TestPrefixHonoursForwardedPrefix(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.Header.Set("X-Forwarded-Prefix", "/agent")
	if got := Prefix(req); got != "/agent/" {
		t.Errorf("Prefix with XFP = %q", got)
	}
}

func TestPreviewPrefixFromPathReturnsEmptyForUnmounted(t *testing.T) {
	if got := previewPrefixFromPath("/foo"); got != "" {
		t.Errorf("previewPrefixFromPath(/foo) = %q, want empty", got)
	}
}
