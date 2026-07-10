package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	authhttp "scaffoldxd1/internal/auth/http"
	authpostgresql "scaffoldxd1/internal/auth/infrastructure/postgresql"
	designapp "scaffoldxd1/internal/design/application"
	designhttp "scaffoldxd1/internal/design/http"
	editorhttp "scaffoldxd1/internal/editor/http"
	gatewayapp "scaffoldxd1/internal/gateway/application"
	gatewayhttp "scaffoldxd1/internal/gateway/http"
	homehttp "scaffoldxd1/internal/home/http"
	testreport "scaffoldxd1/internal/quality/application/test_report"
	qualityhttp "scaffoldxd1/internal/quality/http"
	schedulerhttp "scaffoldxd1/internal/scheduler/http"
	"scaffoldxd1/internal/shared"
	"scaffoldxd1/internal/shared/configuration"
	"scaffoldxd1/internal/shared/infrastructure/postgresql"
	jwks "scaffoldxd1/internal/shared/jwks"
	server "scaffoldxd1/internal/shared/server"
	topologyapp "scaffoldxd1/internal/topology/application"
	topologyhttp "scaffoldxd1/internal/topology/http"
	topologymemory "scaffoldxd1/internal/topology/infrastructure/memory"
	topologymerged "scaffoldxd1/internal/topology/infrastructure/merged"
	topologymutagen "scaffoldxd1/internal/topology/infrastructure/mutagen"
	topologypostgresql "scaffoldxd1/internal/topology/infrastructure/postgresql"
	topologyworkspacefiles "scaffoldxd1/internal/topology/infrastructure/workspacefiles"
	agentapp "scaffoldxd1/pkg/agent/application"
	agenthttp "scaffoldxd1/pkg/agent/http"
	agentdisk "scaffoldxd1/pkg/agent/infrastructure/disk"
	agentmemory "scaffoldxd1/pkg/agent/infrastructure/memory"
	agentpirpc "scaffoldxd1/pkg/agent/infrastructure/pirpc"
	agentpostgresql "scaffoldxd1/pkg/agent/infrastructure/postgresql"
	agentpreview "scaffoldxd1/pkg/agent/infrastructure/preview"
	agentworktree "scaffoldxd1/pkg/agent/infrastructure/worktree"
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
	gatewayhttp.Register(
		s,
		gatewayapp.NewBalanceService(conf.SyncAIGatewayBaseURL, conf.SyncAIGatewayAPIKey),
		gatewayapp.NewSessionCostService(conf.SyncAIGatewayBaseURL, conf.SyncAIGatewayAPIKey),
	)

	// Agente: modo simplificado/legacy. Toda la UI y los endpoints de
	// datos viven en el mismo binario para que air refleje los cambios
	// en una sola app sin depender de BFF + agent-worker.
	agentStore := loadAgentSessionStore()
	workspaceManager := agentworktree.NewManager("", "")
	previewLauncher := agentpreview.NewLauncher()
	manager := agentapp.NewManager(agentStore, agentpirpc.NewRunner(""))
	manager = manager.WithSessionPreparer(func(ctx context.Context, session agentapp.Session) (agentapp.Session, error) {
		prepared, err := workspaceManager.Prepare(ctx, session)
		if err != nil {
			return session, err
		}
		withPreview, err := previewLauncher.Prepare(ctx, prepared)
		if err != nil {
			log.Printf("agent preview: %v", err)
			return prepared, nil
		}
		return withPreview, nil
	}).WithSessionDestroyer(func(ctx context.Context, session agentapp.Session) error {
		if err := previewLauncher.Destroy(ctx, session); err != nil {
			log.Printf("agent preview destroy: %v", err)
		}
		return workspaceManager.Destroy(ctx, session)
	})
	hooks.RegisterShutdown(manager.Close)
	runtimeEventsStore := agentpostgresql.NewRuntimeEventsStore(db)
	agenthttp.SetRuntimeEventsStore(runtimeEventsStore)
	agentapp.SetRuntimeEventsHistorySource(runtimeEventsStore)
	registerAgent(s, manager, sessionRepo, agenthttp.OIDCRefreshConfig{
		TokenEndpoint: conf.OIDCTokenEndpoint,
		ClientID:      conf.OIDCClientID,
		ClientSecret:  conf.OIDCClientSecret,
	})
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

// registerAgent monta la UI del agente y también los endpoints de datos
// en el mismo web-server (wiring legacy) para simplificar dev con air.
func registerAgent(s *server.Server, manager agentapp.AgentService, sessionLookup agenthttp.SessionLookup, oidcCfg agenthttp.OIDCRefreshConfig) {
	agenthttp.RegisterAllLegacy(s, manager, sessionLookup, oidcCfg)
	log.Printf("agent: legacy mode enabled (/agent + data endpoints in cmd/api)")
}

func loadAgentSessionStore() agentapp.SessionStore {
	dir := strings.TrimSpace(os.Getenv("AGENT_SESSION_DIR"))
	if dir == "" {
		dir = "tmp/agent-sessions"
	}
	store, err := agentdisk.NewSessionStore(dir)
	if err == nil {
		log.Printf("agent session store: disk at %s", store.Dir())
		return store
	}
	log.Printf("agent session store: disk unavailable (%v), fallback memory", err)
	return agentmemory.NewSessionStore()
}
