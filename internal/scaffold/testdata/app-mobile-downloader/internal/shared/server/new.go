package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	authpostgresql "gitinittest5/internal/auth/infrastructure/postgresql"
	authmiddleware "gitinittest5/internal/auth/middleware"
	"gitinittest5/internal/shared"
	"gitinittest5/internal/shared/configuration"

	"github.com/MicahParks/keyfunc/v3"
)

type Server struct {
	Mux        http.Handler
	rawMux     *http.ServeMux
	httpServer *http.Server
	middleware []func(http.Handler) http.Handler
	addr       string
}

type ServerOption func(*Server)

func WithAddr(addr string) ServerOption {
	return func(s *Server) { s.addr = addr }
}

func NewServer(opts ...ServerOption) *Server {
	raw := http.NewServeMux()
	s := &Server{rawMux: raw, addr: ":8080"}
	s.Mux = http.HandlerFunc(s.serveHTTP)
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	s.httpServer = &http.Server{Addr: s.addr, Handler: http.HandlerFunc(s.serveHTTP)}
	return s
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	var h http.Handler = s.rawMux
	for i := len(s.middleware) - 1; i >= 0; i-- {
		h = s.middleware[i](h)
	}
	h.ServeHTTP(w, r)
}

func (s *Server) Run() error                         { return s.httpServer.ListenAndServe() }
func (s *Server) Shutdown(ctx context.Context) error { return s.httpServer.Shutdown(ctx) }
func Use(s *Server, mw func(http.Handler) http.Handler) {
	if s == nil || mw == nil {
		return
	}
	s.middleware = append(s.middleware, mw)
}

func New(conf configuration.Conf, jwks keyfunc.Keyfunc, store *authpostgresql.SessionRepository) *Server {
	server := NewServer(WithAddr(":" + strings.TrimSpace(conf.PORT)))
	Use(server, authmiddleware.JWTMiddleware(
		jwks,
		store,
		conf,
	))
	return server
}

func Start(server *Server, hooks *shared.Hooks) error {
	go runServer(server)
	hooks.RegisterShutdown(shutdownHook(server))
	return nil
}

func runServer(server interface{ Run() error }) {
	// http.ErrServerClosed es la salida esperada de http.Server.Serve cuando
	// Shutdown cierra los listeners — no es un fallo. Panicar en ese caso
	// colgaba el shutdown (el test TestStartServerRegistersShutdownHook fallaba
	// por goroutine race con el test runner). Errores reales (bind del puerto,
	// fallo de TLS, etc.) sí panican.
	if err := server.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}

func shutdownHook(server interface{ Shutdown(context.Context) error }) func() error {
	return func() error {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cancel()

		return server.Shutdown(ctx)
	}
}
