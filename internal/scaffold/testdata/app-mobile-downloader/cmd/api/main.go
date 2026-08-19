package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	agentapp "gitinittest5/internal/agent/application"
	agenthttp "gitinittest5/internal/agent/http"
	agentdisk "gitinittest5/internal/agent/infrastructure/disk"
	agentmemory "gitinittest5/internal/agent/infrastructure/memory"
	agentpirpc "gitinittest5/internal/agent/infrastructure/pirpc"
	agenthoncho "gitinittest5/internal/agent/infrastructure/honcho"
	agentpreview "gitinittest5/internal/agent/infrastructure/preview"
	agentworktree "gitinittest5/internal/agent/infrastructure/worktree"
	authhttp "gitinittest5/internal/auth/http"
	authmiddleware "gitinittest5/internal/auth/middleware"
	authpostgresql "gitinittest5/internal/auth/infrastructure/postgresql"
	backlogapp "gitinittest5/internal/backlog/application"
	backlogfs "gitinittest5/internal/backlog/infrastructure/fs"
	backloghttp "gitinittest5/internal/backlog/http"
	designapp "gitinittest5/internal/design/application"
	designhttp "gitinittest5/internal/design/http"
	editorhttp "gitinittest5/internal/editor/http"
	gatewayapp "gitinittest5/internal/gateway/application"
	gatewayhttp "gitinittest5/internal/gateway/http"
	homehttp "gitinittest5/internal/home/http"
	testreport "gitinittest5/internal/quality/application/test_report"
	qualityhttp "gitinittest5/internal/quality/http"
	schedulerhttp "gitinittest5/internal/scheduler/http"
	versionsapp "gitinittest5/internal/versions/application"
	versionshttp "gitinittest5/internal/versions/http"
	"gitinittest5/internal/shared"
	"gitinittest5/internal/shared/configuration"
	"gitinittest5/internal/shared/infrastructure/postgresql"
	jwks "gitinittest5/internal/shared/jwks"
	server "gitinittest5/internal/shared/server"
	topologyapp "gitinittest5/internal/topology/application"
	topologyhttp "gitinittest5/internal/topology/http"
	topologymemory "gitinittest5/internal/topology/infrastructure/memory"
	topologymerged "gitinittest5/internal/topology/infrastructure/merged"
	topologymutagen "gitinittest5/internal/topology/infrastructure/mutagen"
	topologypostgresql "gitinittest5/internal/topology/infrastructure/postgresql"
	topologyworkspacefiles "gitinittest5/internal/topology/infrastructure/workspacefiles"
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
	versionshttp.Register(s, versionsapp.New("."))
	gatewayhttp.Register(
		s,
		gatewayapp.NewBalanceService(conf.SyncAIGatewayBaseURL, conf.SyncAIGatewayAPIKey),
		gatewayapp.NewSessionCostService(conf.SyncAIGatewayBaseURL, conf.SyncAIGatewayAPIKey),
	)
	sessionCostSvc := gatewayapp.NewSessionCostService(conf.SyncAIGatewayBaseURL, conf.SyncAIGatewayAPIKey)

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

	// Wiring del MemoryProvider Honcho (Fase C). Opt-in via
	// HONCHO_ENABLED. Si está apagado o falta la key, el manager
	// conserva su noopProvider default y el comportamiento es
	// idéntico al previo a este cambio.
	if conf.HonchoEnabled && strings.TrimSpace(conf.HonchoAPIKey) != "" {
		adapter, err := agenthoncho.NewAdapter(agenthoncho.Config{
			BaseURL:     conf.HonchoBaseURL,
			APIKey:      conf.HonchoAPIKey,
			WorkspaceID: conf.HonchoWorkspaceID,
			TokenBudget: conf.HonchoTokenBudget,
			SearchTopK:  conf.HonchoTopK,
			RecallTimeout: time.Duration(conf.HonchoRecallTimeoutMS) * time.Millisecond,
		})
		if err != nil {
			log.Printf("honcho: failed to build adapter (%v); continuing with noop provider", err)
		} else {
			manager = manager.WithMemory(adapter).WithMemoryWorkspace(conf.HonchoWorkspaceID)
			log.Printf("honcho: memory provider enabled (workspace=%s base=%s token_budget=%d top_k=%d)",
				conf.HonchoWorkspaceID, conf.HonchoBaseURL, conf.HonchoTokenBudget, conf.HonchoTopK)
		}
	} else {
		log.Printf("honcho: memory provider disabled (HONCHO_ENABLED=%v, key set=%v)",
			conf.HonchoEnabled, strings.TrimSpace(conf.HonchoAPIKey) != "")
	}

	hooks.RegisterShutdown(manager.Close)
	registerAgent(s, manager, sessionRepo, agenthttp.OIDCRefreshConfig{
		TokenEndpoint: conf.OIDCTokenEndpoint,
		ClientID:      conf.OIDCClientID,
		ClientSecret:  conf.OIDCClientSecret,
	}, authmiddleware.RequireEditor(), sessionCostSvc)
	if err := schedulerhttp.Register(s, db, hooks); err != nil {
		log.Fatalf("scheduler: %v", err)
	}

	backlogStore, backlogManager, err := newBacklogDeps()
	if err != nil {
		log.Printf("backlog: %v (continuing without backlog)", err)
	} else {
		_ = backlogStore
		_ = backlogManager
		if err := backloghttp.Register(s, envBacklogDir()); err != nil {
			log.Printf("backlog: register failed: %v", err)
		}
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

// registerAgent monta la UI del agente, los endpoints de datos,
// los assets estáticos del cliente y el worktree inspector en el
// mismo web-server (wiring normal de app única, cmd/api). Tras el
// cutover de 2026-07 toda la superficie del agente vive en una
// sola `agenthttp.Register` (no hay flag ni split V1/V2).
func registerAgent(s *server.Server, manager agentapp.AgentService, sessionLookup agenthttp.SessionLookup, oidcCfg agenthttp.OIDCRefreshConfig, requireEditor func(http.Handler) http.Handler, sessionCostSvc *gatewayapp.SessionCostService) {
	agenthttp.Register(s, manager, sessionLookup, oidcCfg, requireEditor, sessionCostSvc)
}

// newBacklogDeps resuelve el store y manager del backlog. Con el
// perfil OKF v0.1 el store ES el FS; no hay fallback en memoria
// porque OKF no tiene representación alternativa: si el FS no es
// escribible, devolver error y el caller decide qué hacer.
func newBacklogDeps() (backlogapp.Store, *backlogapp.Service, error) {
	dir := envBacklogDir()
	store, err := backlogfs.NewStore(dir)
	if err != nil {
		return nil, nil, err
	}
	log.Printf("backlog store: OKF bundle at %s", store.Root())
	return store, backlogapp.NewService(store), nil
}

// envBacklogDir devuelve el directorio del bundle configurado por
// BACKLOG_DIR, con fallback a internal/backlog/board (datos
// persistentes del módulo internal/backlog).
func envBacklogDir() string {
	dir := strings.TrimSpace(os.Getenv("BACKLOG_DIR"))
	if dir == "" {
		dir = "internal/backlog/board"
	}
	return dir
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
