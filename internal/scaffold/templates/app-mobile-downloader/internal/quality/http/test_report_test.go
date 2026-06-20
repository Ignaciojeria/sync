package dev

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"app-mobile-downloader/internal/quality/ui"

	"github.com/go-fuego/fuego"
)

type fakeContextNoBody struct {
	ctx context.Context
	req *http.Request
	rr  *httptest.ResponseRecorder
	w   http.ResponseWriter
}

type errResponseWriter struct {
	header http.Header
	err    error
}

func (e errResponseWriter) Header() http.Header       { return e.header }
func (e errResponseWriter) WriteHeader(statusCode int) {}
func (e errResponseWriter) Write(p []byte) (int, error) { return 0, e.err }

type errResponseWriterAfter1 struct {
	header http.Header
	err    error
	wrote  bool
}

func (e *errResponseWriterAfter1) Header() http.Header       { return e.header }
func (e *errResponseWriterAfter1) WriteHeader(statusCode int) {}
func (e *errResponseWriterAfter1) Write(p []byte) (int, error) {
	if e.wrote {
		return 0, e.err
	}
	e.wrote = true
	return len(p), nil
}

func newFakeContextNoBody() fakeContextNoBody {
	return fakeContextNoBody{
		ctx: context.Background(),
		req: httptest.NewRequest(http.MethodGet, "/report/tests", nil),
		rr:  httptest.NewRecorder(),
	}
}

func (f fakeContextNoBody) Deadline() (time.Time, bool)               { return time.Time{}, false }
func (f fakeContextNoBody) Done() <-chan struct{}                     { return nil }
func (f fakeContextNoBody) Err() error                                { return nil }
func (f fakeContextNoBody) Value(key any) any                         { return f.ctx.Value(key) }
func (f fakeContextNoBody) Body() (any, error)                        { return nil, nil }
func (f fakeContextNoBody) MustBody() any                             { return nil }
func (f fakeContextNoBody) Params() (any, error)                      { return nil, nil }
func (f fakeContextNoBody) MustParams() any                           { return nil }
func (f fakeContextNoBody) PathParam(name string) string              { return "" }
func (f fakeContextNoBody) PathParamInt(name string) int              { return 0 }
func (f fakeContextNoBody) PathParamIntErr(name string) (int, error)  { return 0, nil }
func (f fakeContextNoBody) QueryParam(name string) string             { return "" }
func (f fakeContextNoBody) QueryParamArr(name string) []string        { return nil }
func (f fakeContextNoBody) QueryParamInt(name string) int             { return 0 }
func (f fakeContextNoBody) QueryParamIntErr(name string) (int, error) { return 0, nil }
func (f fakeContextNoBody) QueryParamBool(name string) bool           { return false }
func (f fakeContextNoBody) QueryParamBoolErr(name string) (bool, error) { return false, nil }
func (f fakeContextNoBody) QueryParams() url.Values                   { return url.Values{} }
func (f fakeContextNoBody) MainLang() string                          { return "" }
func (f fakeContextNoBody) MainLocale() string                        { return "" }
func (f fakeContextNoBody) Render(templateToExecute string, data any, templateGlobsToOverride ...string) (fuego.CtxRenderer, error) {
	return nil, nil
}
func (f fakeContextNoBody) Cookie(name string) (*http.Cookie, error) { return f.req.Cookie(name) }
func (f fakeContextNoBody) SetCookie(cookie http.Cookie)             { http.SetCookie(f.rr, &cookie) }
func (f fakeContextNoBody) Header(key string) string                 { return f.req.Header.Get(key) }
func (f fakeContextNoBody) SetHeader(key, value string)              { f.rr.Header().Set(key, value) }
func (f fakeContextNoBody) Context() context.Context                 { return f.ctx }
func (f fakeContextNoBody) Request() *http.Request                   { return f.req }
func (f fakeContextNoBody) Response() http.ResponseWriter {
	if f.w != nil {
		return f.w
	}
	return f.rr
}
func (f fakeContextNoBody) SetStatus(code int)                       { f.rr.WriteHeader(code) }
func (f fakeContextNoBody) Redirect(code int, target string) (any, error) {
	http.Redirect(f.rr, f.req, target, code)
	return nil, nil
}
func (f fakeContextNoBody) GetOpenAPIParams() map[string]fuego.OpenAPIParam { return nil }
func (f fakeContextNoBody) HasQueryParam(key string) bool                    { return false }
func (f fakeContextNoBody) HasHeader(key string) bool                        { return f.req.Header.Get(key) != "" }
func (f fakeContextNoBody) HasCookie(key string) bool {
	_, err := f.req.Cookie(key)
	return err == nil
}

func TestRenderResultAndDashboard(t *testing.T) {
	c := newFakeContextNoBody()
	state := ui.TestRunState{
		Success:      true,
		Output:       "ok",
		CoverPath:    "/report/tests/coverage.html?t=123",
		CoverPercent: 88.8,
		Timestamp:    time.Unix(1700000000, 0).UTC(),
		HasResult:    true,
	}
	if _, err := renderResultAndDashboard(c, state); err != nil {
		t.Fatalf("renderResultAndDashboard() error = %v", err)
	}
	if got := c.rr.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content-type = %q", got)
	}
	body := c.rr.Body.String()
	if !strings.Contains(body, "88.8%") || !strings.Contains(body, "Ver reporte") {
		t.Fatalf("unexpected rendered body: %q", body)
	}

	failing := newFakeContextNoBody()
	failing.w = errResponseWriter{header: http.Header{}, err: errors.New("write failed")}
	if _, err := renderResultAndDashboard(failing, state); err == nil {
		t.Fatal("expected render error when writer fails")
	}
}

func TestRenderResultAndDashboardFirstTemplateError(t *testing.T) {
	c := newFakeContextNoBody()
	c.w = errResponseWriter{header: http.Header{}, err: errors.New("write failed")}
	if _, err := renderResultAndDashboard(c, ui.TestRunState{}); err == nil {
		t.Fatal("expected render error when writer fails on first template")
	}
}

func TestRenderResultAndDashboardSecondTemplateError(t *testing.T) {
	c := newFakeContextNoBody()
	c.w = &errResponseWriterAfter1{header: http.Header{}, err: errors.New("write failed")}
	state := ui.TestRunState{Success: true, Output: "ok", CoverPath: "/c", CoverPercent: 50}
	if _, err := renderResultAndDashboard(c, state); err == nil {
		t.Fatal("expected render error when writer fails on second template")
	}
}


