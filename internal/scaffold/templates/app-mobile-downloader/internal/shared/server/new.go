package server

import (
	"context"
	"strings"
	"time"

	authmiddleware "app-mobile-downloader/internal/auth/middleware"
	authpostgresql "app-mobile-downloader/internal/auth/infrastructure/postgresql"
	"app-mobile-downloader/internal/shared/configuration"

	"github.com/Ignaciojeria/ioc"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(New)
var _ = ioc.Register(startServer)

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

func startServer(
	server *Server,
	shutdowner ioc.Shutdowner,
) error {
	go runServer(server)
	shutdowner.RegisterShutdown(shutdownHook(server))
	return nil
}

func runServer(server interface{ Run() error }) {
	if err := server.Run(); err != nil {
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


