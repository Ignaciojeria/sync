package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	authmiddleware "testboi1/internal/auth/middleware"
	authpostgresql "testboi1/internal/auth/infrastructure/postgresql"
	"testboi1/internal/shared"
	"testboi1/internal/shared/configuration"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/go-fuego/fuego"
)

type Server struct {
	*fuego.Server
}

func New(conf configuration.Conf, jwks keyfunc.Keyfunc, store *authpostgresql.SessionRepository) *Server {
	server := fuego.NewServer(fuego.WithAddr(":" + strings.TrimSpace(conf.PORT)))
	fuego.Use(server, authmiddleware.JWTMiddleware(
		jwks,
		store,
		conf,
	))
	return &Server{Server: server}
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
