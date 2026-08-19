package layout

import (
	"bytes"
	"context"
	"gitinittest5/internal/shared/server"
	"github.com/a-h/templ"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// minimalCtx is a tiny stand-in for server.ContextNoBody that only satisfies
// the methods RenderPage touches. Other fuego tests construct a fuller impl.
type minimalCtx struct {
	req *http.Request
	w   http.ResponseWriter
}

func (m minimalCtx) Deadline() (deadline time.Time, ok bool) { return }
func (m minimalCtx) Done() <-chan struct{}                   { return nil }
func (m minimalCtx) Err() error                              { return nil }
func (m minimalCtx) Value(key any) any                       { return m.req.Context().Value(key) }
func (m minimalCtx) Body() (any, error)                      { return nil, nil }
func (m minimalCtx) MustBody() any                           { return nil }
func (m minimalCtx) Params() (any, error)                    { return nil, nil }
func (m minimalCtx) MustParams() any                         { return nil }
func (m minimalCtx) PathParam(string) string                 { return "" }
func (m minimalCtx) PathParamInt(string) int                 { return 0 }
func (m minimalCtx) PathParamIntErr(string) (int, error)     { return 0, nil }
func (m minimalCtx) QueryParam(string) string                { return "" }
func (m minimalCtx) QueryParamArr(string) []string           { return nil }
func (m minimalCtx) QueryParamInt(string) int                { return 0 }
func (m minimalCtx) QueryParamIntErr(string) (int, error)    { return 0, nil }
func (m minimalCtx) QueryParamBool(string) bool              { return false }
func (m minimalCtx) QueryParamBoolErr(string) (bool, error)  { return false, nil }
func (m minimalCtx) QueryParams() url.Values {
	return m.req.URL.Query()
}
func (m minimalCtx) MainLang() string   { return "" }
func (m minimalCtx) MainLocale() string { return "" }
func (m minimalCtx) Render(string, any, ...string) (server.CtxRenderer, error) {
	return nil, nil
}
func (m minimalCtx) Cookie(string) (*http.Cookie, error)              { return m.req.Cookie("") }
func (m minimalCtx) SetCookie(http.Cookie)                            {}
func (m minimalCtx) Header(string) string                             { return m.req.Header.Get("") }
func (m minimalCtx) SetHeader(key, value string)                      {}
func (m minimalCtx) Context() context.Context                         { return m.req.Context() }
func (m minimalCtx) Request() *http.Request                           { return m.req }
func (m minimalCtx) Response() http.ResponseWriter                    { return m.w }
func (m minimalCtx) SetStatus(int)                                    {}
func (m minimalCtx) Redirect(int, string) (any, error)                { return nil, nil }
func (m minimalCtx) GetOpenAPIParams() map[string]server.OpenAPIParam { return nil }
func (m minimalCtx) HasQueryParam(string) bool                        { return false }
func (m minimalCtx) HasHeader(string) bool                            { return false }
func (m minimalCtx) HasCookie(string) bool                            { return false }

// Verify minimalCtx implements server.ContextNoBody.
var _ server.ContextNoBody = minimalCtx{}

func TestRenderPageInvokesLayoutAndContent(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	c := minimalCtx{req: req, w: rec}

	content := templ.Raw("hello-content")

	page, err := RenderPage(c, "My Title", content)
	if err != nil {
		t.Fatalf("RenderPage() error = %v", err)
	}

	var buf bytes.Buffer
	if err := page.Render(context.Background(), &buf); err != nil {
		t.Fatalf("page.Render() error = %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, "My Title") {
		t.Fatalf("expected rendered title, got %q", body)
	}
	if !strings.Contains(body, "hello-content") {
		t.Fatalf("expected rendered content, got %q", body)
	}
	if !strings.Contains(body, "drawer") {
		t.Fatalf("expected drawer layout, got %q", body)
	}
}

func TestRenderPublicPageSkipsDrawer(t *testing.T) {
	req := httptest.NewRequest("GET", "/auth/login", nil)
	rec := httptest.NewRecorder()
	c := minimalCtx{req: req, w: rec}

	content := templ.Raw("public-content")

	page, err := RenderPublicPage(c, "Login", content)
	if err != nil {
		t.Fatalf("RenderPublicPage() error = %v", err)
	}

	var buf bytes.Buffer
	if err := page.Render(context.Background(), &buf); err != nil {
		t.Fatalf("page.Render() error = %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, "Login") {
		t.Fatalf("expected title, got %q", body)
	}
	if !strings.Contains(body, "public-content") {
		t.Fatalf("expected content, got %q", body)
	}
	// Layout público no debe envolver con sidenav.
	if strings.Contains(body, "drawer") {
		t.Fatalf("public layout no debe tener drawer, got %q", body)
	}
}
