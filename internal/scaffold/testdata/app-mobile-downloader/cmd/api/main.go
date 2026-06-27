package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	authhttp "app-mobile-downloader/internal/auth/http"
	authpostgresql "app-mobile-downloader/internal/auth/infrastructure/postgresql"
	designapp "app-mobile-downloader/internal/design/application"
	designhttp "app-mobile-downloader/internal/design/http"
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
	topologyapp "app-mobile-downloader/internal/topology/application"
	topologyhttp "app-mobile-downloader/internal/topology/http"
	topologymemory "app-mobile-downloader/internal/topology/infrastructure/memory"
	topologymerged "app-mobile-downloader/internal/topology/infrastructure/merged"
	topologymutagen "app-mobile-downloader/internal/topology/infrastructure/mutagen"
	topologypostgresql "app-mobile-downloader/internal/topology/infrastructure/postgresql"
	topologyworkspacefiles "app-mobile-downloader/internal/topology/infrastructure/workspacefiles"
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

	designCatalog, err := designapp.DefaultCatalog()
	if err != nil {
		log.Fatalf("design catalog: %v", err)
	}

	testRunner := testreport.NewRunner()
	syncSessionsStore := topologymemory.NewSyncSessionsStore(2 * time.Minute)
	syncSessionsSource := topologymerged.NewSource(
		topologymutagen.NewSource(),
		topologyworkspacefiles.NewSource(".einar/sessions", 2*time.Minute),
		syncSessionsStore,
	)
	topologyService := topologyapp.NewServiceWithDeps(topologyapp.ServiceDeps{
		WorkspaceName:      conf.PROJECT_NAME,
		WorkspaceSummary:   "Runtime persistente del workspace con sesiones de sync reportadas y servicios activos.",
		ServicesSource:     topologypostgresql.NewSource(db),
		SyncSessionsSource: syncSessionsSource,
		SyncSessionsStore:  syncSessionsStore,
	})

	designhttp.Register(s, designCatalog)
	homehttp.Register(s, topologyService)
	topologyhttp.Register(s, topologyService)
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
