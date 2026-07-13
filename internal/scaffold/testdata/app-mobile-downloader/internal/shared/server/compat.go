package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type CtxRenderer interface{}
type OpenAPIParam struct{}

type HTTPError struct {
	Status int
	Title  string
	Detail string
}

func (e HTTPError) Error() string {
	if strings.TrimSpace(e.Detail) != "" {
		return e.Detail
	}
	if e.Status > 0 {
		return http.StatusText(e.Status)
	}
	return "http error"
}

type ContextNoBody interface {
	context.Context
	Body() (any, error)
	MustBody() any
	Params() (any, error)
	MustParams() any
	PathParam(string) string
	PathParamInt(string) int
	PathParamIntErr(string) (int, error)
	QueryParam(string) string
	QueryParamArr(string) []string
	QueryParamInt(string) int
	QueryParamIntErr(string) (int, error)
	QueryParamBool(string) bool
	QueryParamBoolErr(string) (bool, error)
	QueryParams() url.Values
	MainLang() string
	MainLocale() string
	Render(string, any, ...string) (CtxRenderer, error)
	Cookie(string) (*http.Cookie, error)
	SetCookie(http.Cookie)
	Header(string) string
	SetHeader(string, string)
	Context() context.Context
	Request() *http.Request
	Response() http.ResponseWriter
	SetStatus(int)
	Redirect(int, string) (any, error)
	GetOpenAPIParams() map[string]OpenAPIParam
	HasQueryParam(string) bool
	HasHeader(string) bool
	HasCookie(string) bool
}

type runtimeContext struct {
	req *http.Request
	res http.ResponseWriter
}

func (c *runtimeContext) Deadline() (time.Time, bool) { return c.req.Context().Deadline() }
func (c *runtimeContext) Done() <-chan struct{}       { return c.req.Context().Done() }
func (c *runtimeContext) Err() error                  { return c.req.Context().Err() }
func (c *runtimeContext) Value(key any) any           { return c.req.Context().Value(key) }
func (c *runtimeContext) Body() (any, error)          { return nil, nil }
func (c *runtimeContext) MustBody() any               { return nil }
func (c *runtimeContext) Params() (any, error)        { return nil, nil }
func (c *runtimeContext) MustParams() any             { return nil }
func (c *runtimeContext) PathParam(name string) string {
	return strings.TrimSpace(c.req.PathValue(name))
}
func (c *runtimeContext) PathParamInt(name string) int {
	n, _ := c.PathParamIntErr(name)
	return n
}
func (c *runtimeContext) PathParamIntErr(name string) (int, error) {
	return strconv.Atoi(c.PathParam(name))
}
func (c *runtimeContext) QueryParam(name string) string {
	return strings.TrimSpace(c.req.URL.Query().Get(name))
}
func (c *runtimeContext) QueryParamArr(name string) []string { return c.req.URL.Query()[name] }
func (c *runtimeContext) QueryParamInt(name string) int {
	n, _ := c.QueryParamIntErr(name)
	return n
}
func (c *runtimeContext) QueryParamIntErr(name string) (int, error) {
	return strconv.Atoi(c.QueryParam(name))
}
func (c *runtimeContext) QueryParamBool(name string) bool {
	b, _ := c.QueryParamBoolErr(name)
	return b
}
func (c *runtimeContext) QueryParamBoolErr(name string) (bool, error) {
	return strconv.ParseBool(c.QueryParam(name))
}
func (c *runtimeContext) QueryParams() url.Values                            { return c.req.URL.Query() }
func (c *runtimeContext) MainLang() string                                   { return "" }
func (c *runtimeContext) MainLocale() string                                 { return "" }
func (c *runtimeContext) Render(string, any, ...string) (CtxRenderer, error) { return nil, nil }
func (c *runtimeContext) Cookie(name string) (*http.Cookie, error)           { return c.req.Cookie(name) }
func (c *runtimeContext) SetCookie(cookie http.Cookie)                       { http.SetCookie(c.res, &cookie) }
func (c *runtimeContext) Header(key string) string                           { return c.req.Header.Get(key) }
func (c *runtimeContext) SetHeader(key, value string)                        { c.res.Header().Set(key, value) }
func (c *runtimeContext) Context() context.Context                           { return c.req.Context() }
func (c *runtimeContext) Request() *http.Request                             { return c.req }
func (c *runtimeContext) Response() http.ResponseWriter                      { return c.res }
func (c *runtimeContext) SetStatus(code int)                                 { c.res.WriteHeader(code) }
func (c *runtimeContext) Redirect(code int, target string) (any, error) {
	http.Redirect(c.res, c.req, target, code)
	return nil, nil
}
func (c *runtimeContext) GetOpenAPIParams() map[string]OpenAPIParam { return nil }
func (c *runtimeContext) HasQueryParam(key string) bool             { _, ok := c.req.URL.Query()[key]; return ok }
func (c *runtimeContext) HasHeader(key string) bool {
	return strings.TrimSpace(c.req.Header.Get(key)) != ""
}
func (c *runtimeContext) HasCookie(key string) bool {
	_, err := c.req.Cookie(key)
	return err == nil
}

type MockContextNoBody struct {
	Req        *http.Request
	RR         *httptest.ResponseRecorder
	W          http.ResponseWriter
	PathParams map[string]string
}

func NewMockContextNoBody() *MockContextNoBody {
	return &MockContextNoBody{
		Req:        httptest.NewRequest(http.MethodGet, "/", nil),
		RR:         httptest.NewRecorder(),
		PathParams: map[string]string{},
	}
}

func (m *MockContextNoBody) SetRequest(r *http.Request) {
	if r != nil {
		m.Req = r
	}
}

func (m *MockContextNoBody) Deadline() (time.Time, bool) { return m.Req.Context().Deadline() }
func (m *MockContextNoBody) Done() <-chan struct{}       { return m.Req.Context().Done() }
func (m *MockContextNoBody) Err() error                  { return m.Req.Context().Err() }
func (m *MockContextNoBody) Value(key any) any           { return m.Req.Context().Value(key) }
func (m *MockContextNoBody) Body() (any, error)          { return nil, nil }
func (m *MockContextNoBody) MustBody() any               { return nil }
func (m *MockContextNoBody) Params() (any, error)        { return nil, nil }
func (m *MockContextNoBody) MustParams() any             { return nil }
func (m *MockContextNoBody) PathParam(name string) string {
	if v := strings.TrimSpace(m.PathParams[name]); v != "" {
		return v
	}
	return strings.TrimSpace(m.Req.PathValue(name))
}
func (m *MockContextNoBody) PathParamInt(name string) int {
	n, _ := m.PathParamIntErr(name)
	return n
}
func (m *MockContextNoBody) PathParamIntErr(name string) (int, error) {
	return strconv.Atoi(m.PathParam(name))
}
func (m *MockContextNoBody) QueryParam(name string) string {
	return strings.TrimSpace(m.Req.URL.Query().Get(name))
}
func (m *MockContextNoBody) QueryParamArr(name string) []string { return m.Req.URL.Query()[name] }
func (m *MockContextNoBody) QueryParamInt(name string) int {
	n, _ := m.QueryParamIntErr(name)
	return n
}
func (m *MockContextNoBody) QueryParamIntErr(name string) (int, error) {
	return strconv.Atoi(m.QueryParam(name))
}
func (m *MockContextNoBody) QueryParamBool(name string) bool {
	b, _ := m.QueryParamBoolErr(name)
	return b
}
func (m *MockContextNoBody) QueryParamBoolErr(name string) (bool, error) {
	return strconv.ParseBool(m.QueryParam(name))
}
func (m *MockContextNoBody) QueryParams() url.Values                            { return m.Req.URL.Query() }
func (m *MockContextNoBody) MainLang() string                                   { return "" }
func (m *MockContextNoBody) MainLocale() string                                 { return "" }
func (m *MockContextNoBody) Render(string, any, ...string) (CtxRenderer, error) { return nil, nil }
func (m *MockContextNoBody) Cookie(name string) (*http.Cookie, error)           { return m.Req.Cookie(name) }
func (m *MockContextNoBody) SetCookie(cookie http.Cookie)                       { http.SetCookie(m.Response(), &cookie) }
func (m *MockContextNoBody) Header(key string) string                           { return m.Req.Header.Get(key) }
func (m *MockContextNoBody) SetHeader(key, value string)                        { m.Response().Header().Set(key, value) }
func (m *MockContextNoBody) Context() context.Context                           { return m.Req.Context() }
func (m *MockContextNoBody) Request() *http.Request                             { return m.Req }
func (m *MockContextNoBody) Response() http.ResponseWriter {
	if m.W != nil {
		return m.W
	}
	return m.RR
}
func (m *MockContextNoBody) SetStatus(code int) { m.Response().WriteHeader(code) }
func (m *MockContextNoBody) Redirect(code int, target string) (any, error) {
	http.Redirect(m.Response(), m.Req, target, code)
	return nil, nil
}
func (m *MockContextNoBody) GetOpenAPIParams() map[string]OpenAPIParam { return nil }
func (m *MockContextNoBody) HasQueryParam(key string) bool {
	_, ok := m.Req.URL.Query()[key]
	return ok
}
func (m *MockContextNoBody) HasHeader(key string) bool {
	return strings.TrimSpace(m.Req.Header.Get(key)) != ""
}
func (m *MockContextNoBody) HasCookie(key string) bool {
	_, err := m.Req.Cookie(key)
	return err == nil
}

type routeOptions struct {
	middleware []func(http.Handler) http.Handler
}
type RouteOption func(*routeOptions)

func OptionMiddleware(mw func(http.Handler) http.Handler) RouteOption {
	return func(o *routeOptions) {
		if mw != nil {
			o.middleware = append(o.middleware, mw)
		}
	}
}

func applyRouteOptions(h http.Handler, opts []RouteOption) http.Handler {
	var ro routeOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&ro)
		}
	}
	for i := len(ro.middleware) - 1; i >= 0; i-- {
		h = ro.middleware[i](h)
	}
	return h
}

func writeResult(w http.ResponseWriter, v any) {
	if v == nil {
		return
	}
	switch x := v.(type) {
	case string:
		if x == "" {
			return
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
		_, _ = io.WriteString(w, x)
	case []byte:
		_, _ = w.Write(x)
	default:
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	detail := strings.TrimSpace(err.Error())
	if he, ok := err.(HTTPError); ok {
		if he.Status > 0 {
			status = he.Status
		}
		if strings.TrimSpace(he.Detail) != "" {
			detail = he.Detail
		}
	} else if he, ok := any(err).(*HTTPError); ok && he != nil {
		if he.Status > 0 {
			status = he.Status
		}
		if strings.TrimSpace(he.Detail) != "" {
			detail = he.Detail
		}
	}
	if detail == "" {
		detail = http.StatusText(status)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"detail": detail})
}

func adaptNoBodyT[T any](fn func(ContextNoBody) (T, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, err := fn(&runtimeContext{req: r, res: w})
		if err != nil {
			writeError(w, err)
			return
		}
		writeResult(w, any(v))
	})
}

func Get[T any](s *Server, path string, fn func(ContextNoBody) (T, error), opts ...RouteOption) {
	register(s, http.MethodGet+" "+path, adaptNoBodyT(fn), opts...)
}

func Post[T any](s *Server, path string, fn func(ContextNoBody) (T, error), opts ...RouteOption) {
	register(s, http.MethodPost+" "+path, adaptNoBodyT(fn), opts...)
}

func Delete[T any](s *Server, path string, fn func(ContextNoBody) (T, error), opts ...RouteOption) {
	register(s, http.MethodDelete+" "+path, adaptNoBodyT(fn), opts...)
}

func All[T any](s *Server, path string, fn func(ContextNoBody) (T, error), opts ...RouteOption) {
	register(s, path, adaptNoBodyT(fn), opts...)
}

func Handle(s *Server, path string, h http.Handler, opts ...RouteOption) {
	register(s, path, h, opts...)
}

func register(s *Server, pattern string, h http.Handler, opts ...RouteOption) {
	if s == nil {
		panic("nil server")
	}
	s.rawMux.Handle(pattern, applyRouteOptions(h, opts))
}
