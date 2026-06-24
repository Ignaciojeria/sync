package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	authpostgresql "app-mobile-downloader/internal/auth/infrastructure/postgresql"
	authhttp "app-mobile-downloader/internal/auth/http"
	editorhttp "app-mobile-downloader/internal/editor/http"
	homehttp "app-mobile-downloader/internal/home/http"
	testreport "app-mobile-downloader/internal/quality/application/test_report"
	qualityhttp "app-mobile-downloader/internal/quality/http"
	schedulerhttp "app-mobile-downloader/internal/scheduler/http"
	"app-mobile-downloader/internal/shared"
	"app-mobile-downloader/internal/shared/configuration"
	"app-mobile-downloader/internal/shared/infrastructure/postgresql"
	jwks "app-mobile-downloader/internal/shared/jwks"
	server "app-mobile-downloader/internal/shared/server"
)

func main() {
	conf, err := configuration.NewConf()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := postgresql.NewConnection(conf)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}

	k, err := jwks.New(conf)
	if err != nil {
		log.Fatalf("jwks: %v", err)
	}

	sessionRepo := authpostgresql.NewSessionRepository(db)

	s := server.New(conf, k, sessionRepo)
	hooks := &shared.Hooks{}

	testRunner := testreport.NewRunner()

	homehttp.Register(s)
	editorhttp.Register(s)
	authhttp.Register(s, conf, sessionRepo, k)
	qualityhttp.Register(s, testRunner)
	if err := schedulerhttp.Register(s, db, hooks); err != nil {
		log.Fatalf("scheduler: %v", err)
	}

	if err := server.Start(s, hooks); err != nil {
		log.Fatalf("server start: %v", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	if err := hooks.Shutdown(context.Background()); err != nil {
		log.Printf("shutdown completed with errors: %v", err)
	}
}
