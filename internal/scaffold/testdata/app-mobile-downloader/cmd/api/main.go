package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	authhttp "testboi1/internal/auth/http"
	authpostgresql "testboi1/internal/auth/infrastructure/postgresql"
	designapp "testboi1/internal/design/application"
	designhttp "testboi1/internal/design/http"
	editorhttp "testboi1/internal/editor/http"
	gatewayapp "testboi1/internal/gateway/application"
	gatewayhttp "testboi1/internal/gateway/http"
	homehttp "testboi1/internal/home/http"
	testreport "testboi1/internal/quality/application/test_report"
	qualityhttp "testboi1/internal/quality/http"
	schedulerhttp "testboi1/internal/scheduler/http"
	"testboi1/internal/shared"
	"testboi1/internal/shared/configuration"
	"testboi1/internal/shared/infrastructure/postgresql"
	jwks "testboi1/internal/shared/jwks"
	server "testboi1/internal/shared/server"
	topologyapp "testboi1/internal/topology/application"
	topologyhttp "testboi1/internal/topology/http"
	topologymemory "testboi1/internal/topology/infrastructure/memory"
	topologymerged "testboi1/internal/topology/infrastructure/merged"
	topologymutagen "testboi1/internal/topology/infrastructure/mutagen"
	topologypostgresql "testboi1/internal/topology/infrastructure/postgresql"
	topologyworkspacefiles "testboi1/internal/topology/infrastructure/workspacefiles"
	agentapp "testboi1/pkg/agent/application"
	agenthttp "testboi1/pkg/agent/http"
	agentdisk "testboi1/pkg/agent/infrastructure/disk"
	agentmemory "testboi1/pkg/agent/infrastructure/memory"
	agentpirpc "testboi1/pkg/agent/infrastructure/pirpc"
	agentpostgresql "testboi1/pkg/agent/infrastructure/postgresql"
	agentpreview "testboi1/pkg/agent/infrastructure/preview"
	agentworktree "testboi1/pkg/agent/infrastructure/worktree"
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
