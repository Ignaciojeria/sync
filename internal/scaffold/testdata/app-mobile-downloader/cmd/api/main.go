package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	agentapp "fixtests1/internal/agent/application"
	agenthttp "fixtests1/internal/agent/http"
	agentdisk "fixtests1/internal/agent/infrastructure/disk"
	agentmemory "fixtests1/internal/agent/infrastructure/memory"
	agentpirpc "fixtests1/internal/agent/infrastructure/pirpc"
	agentpostgresql "fixtests1/internal/agent/infrastructure/postgresql"
	agentpreview "fixtests1/internal/agent/infrastructure/preview"
	agentworktree "fixtests1/internal/agent/infrastructure/worktree"
	authhttp "fixtests1/internal/auth/http"
	authpostgresql "fixtests1/internal/auth/infrastructure/postgresql"
	designapp "fixtests1/internal/design/application"
	designhttp "fixtests1/internal/design/http"
	editorhttp "fixtests1/internal/editor/http"
	gatewayapp "fixtests1/internal/gateway/application"
	gatewayhttp "fixtests1/internal/gateway/http"
	homehttp "fixtests1/internal/home/http"
	testreport "fixtests1/internal/quality/application/test_report"
	qualityhttp "fixtests1/internal/quality/http"
	schedulerhttp "fixtests1/internal/scheduler/http"
	"fixtests1/internal/shared"
	"fixtests1/internal/shared/configuration"
	"fixtests1/internal/shared/infrastructure/postgresql"
	jwks "fixtests1/internal/shared/jwks"
	server "fixtests1/internal/shared/server"
	topologyapp "fixtests1/internal/topology/application"
	topologyhttp "fixtests1/internal/topology/http"
	topologymemory "fixtests1/internal/topology/infrastructure/memory"
	topologymerged "fixtests1/internal/topology/infrastructure/merged"
	topologymutagen "fixtests1/internal/topology/infrastructure/mutagen"
	topologypostgresql "fixtests1/internal/topology/infrastructure/postgresql"
	topologyworkspacefiles "fixtests1/internal/topology/infrastructure/workspacefiles"
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
		// Matar watchers/procesos cuyo CWD esté dentro del workspace
		// de la sesión antes de borrarlo. Best-effort: si falla o
		// devuelve 0, no bloqueamos la destrucción.
		if killed, err := agentapp.KillProcessesInWorkspace(ctx, session.WorkspacePath); err != nil {
			log.Printf("agent workspace cleanup (%s): %v", session.ID, err)
		} else if killed > 0 {
			log.Printf("agent workspace cleanup (%s): killed %d process(es)", session.ID, killed)
		}
		return workspaceManager.Destroy(ctx, session)
	}).WithSessionApplier(func(ctx context.Context, session agentapp.Session) (agentapp.ApplyResult, error) {
		return workspaceManager.ApplyPreview(ctx, session)
	}).WithSessionMerger(func(ctx context.Context, session agentapp.Session) (agentapp.MergeResult, error) {
		return workspaceManager.MergePreview(ctx, session)
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
