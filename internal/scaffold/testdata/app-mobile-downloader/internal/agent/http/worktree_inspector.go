package agent

import (
	"net/http"
	"strings"

	agentapp "lastmile-agents/internal/agent/application"
	agentuiv2 "lastmile-agents/internal/agent/ui/v2"
	agentworktree "lastmile-agents/internal/agent/infrastructure/worktree"
	"lastmile-agents/internal/shared/server"
)

// ponytail: endpoints del Worktree Inspector Panel. Lee metadata
// del worktree (branch, commits, files changed, diffs) y lo
// devuelve como JSON al cliente.
//
// Endpoints:
//   GET  /agent/sessions/{id}/worktree
//        → snapshot completo (branch, files, commits, stats).
//   GET  /agent/sessions/{id}/worktree/diff?file=PATH
//        → diff completo de un archivo especifico.
//   POST /agent/sessions/{id}/worktree/apply
//        → mergea el worktree a la rama principal.
//
// Antes vivían en /agent-v2/ (cliente V2 con shell separada). Tras
// el cutover 2026-07 la V2 pasó a /agent y estos endpoints la
// acompañan. El handler HTTP es un forward al servicio real que
// vive en infrastructure/worktree.
func worktreeInspectorHandler(s *server.Server, manager agentapp.AgentService, inspector *agentworktree.Inspector, requireEditor func(http.Handler) http.Handler) {
	server.Handle(s, "GET /agent/sessions/{id}/worktree", requireEditor(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID, err := pathSessionID(r)
		if err != nil {
			writeError(w, err)
			return
		}
		if !isSafeSessionID(sessionID) {
			writeError(w, server.HTTPError{Status: http.StatusBadRequest, Detail: "invalid session id"})
			return
		}
		agentuiv2.RegisterRendererForSession(sessionID)
		sess, err := manager.Get(r.Context(), sessionID)
		if err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		snap, err := inspector.Inspect(r.Context(), sess)
		if err != nil {
			// ponytail: si la sesion no tiene worktree todavia
			// (caso comun cuando recien se creo), devolvemos
			// 200 con un snapshot vacio en vez de error.
			// Asi el panel puede mostrar "Sin cambios" en lugar
			// de romperse.
			writeJSON(w, http.StatusOK, agentworktree.WorktreeSnapshot{
				Branch:     strings.TrimSpace(sess.Branch),
				BaseBranch: strings.TrimSpace(sess.BaseBranch),
				Files:      []agentworktree.FileChange{},
				Commits:    []agentworktree.CommitEntry{},
			})
			return
		}
		writeJSON(w, http.StatusOK, snap)
	})))
	server.Handle(s, "GET /agent/sessions/{id}/worktree/diff", requireEditor(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID, err := pathSessionID(r)
		if err != nil {
			writeError(w, err)
			return
		}
		if !isSafeSessionID(sessionID) {
			writeError(w, server.HTTPError{Status: http.StatusBadRequest, Detail: "invalid session id"})
			return
		}
		filePath := strings.TrimSpace(r.URL.Query().Get("file"))
		if filePath == "" {
			writeError(w, server.HTTPError{Status: http.StatusBadRequest, Detail: "file query param is required"})
			return
		}
		agentuiv2.RegisterRendererForSession(sessionID)
		sess, err := manager.Get(r.Context(), sessionID)
		if err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		diff, err := inspector.Diff(r.Context(), sess, filePath)
		if err != nil {
			writeError(w, server.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, diff)
	})))
}
