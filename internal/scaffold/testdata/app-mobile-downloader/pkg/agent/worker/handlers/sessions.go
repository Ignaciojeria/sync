package handlers

import (
	"net/http"
	"strings"

	agentapp "scaffoldxd1/pkg/agent/application"
)

// Register cuelga TODOS los endpoints de datos del agente en mux.
// requireRequire es el middleware que valida X-Internal-Auth; se aplica
// a cada uno.
//
// Rutas expuestas:
//
//	GET    /agent/sessions                  -> listar sesiones
//	POST   /agent/sessions                  -> crear sesión
//	GET    /agent/sessions/<id>             -> obtener sesión
//	POST   /agent/sessions/<id>/prompt      -> enviar prompt
//	POST   /agent/sessions/<id>/steer       -> enviar steer
//	POST   /agent/sessions/<id>/abort       -> abortar turn
//	GET    /agent/sessions/<id>/events      -> SSE stream
//
// La página completa (GET /agent) sigue en el web-server porque
// depende de templ + layout.
func Register(mux *http.ServeMux, mgr agentapp.AgentService, requireRequire func(http.Handler) http.Handler) {
	// Sesiones CRUD: /agent/sessions exacto.
	mux.Handle("/agent/sessions", requireRequire(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListSessions(w, r, mgr)
		case http.MethodPost:
			handleCreateSession(w, r, mgr)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})))

	// Sub-rutas de /agent/sessions/: prompt/steer/abort/events/get.
	// Un único handler dispatchea por sufijo.
	mux.Handle("/agent/sessions/", requireRequire(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := extractSessionID(r)
		if id == "" {
			http.Error(w, "missing session id", http.StatusBadRequest)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/prompt") && r.Method == http.MethodPost:
			handlePrompt(w, r, mgr, id)
		case strings.HasSuffix(r.URL.Path, "/steer") && r.Method == http.MethodPost:
			handleSteer(w, r, mgr, id)
		case strings.HasSuffix(r.URL.Path, "/abort") && r.Method == http.MethodPost:
			handleAbort(w, r, mgr, id)
		case strings.HasSuffix(r.URL.Path, "/events") && r.Method == http.MethodGet:
			handleEvents(w, r, mgr, id)
		case strings.HasSuffix(r.URL.Path, "/history") && r.Method == http.MethodGet:
			handleHistory(w, r, mgr, id)
		case r.URL.Path == "/agent/sessions/"+id && r.Method == http.MethodGet:
			handleGetSession(w, r, mgr, id)
		default:
			http.NotFound(w, r)
		}
	})))

	// Inventario de procesos `pi` corriendo. Vive en el worker (que
	// tiene visibilidad directa sobre /proc del host donde spawnea las
	// runtimes). El web-server no necesita proxy: la UI lo consume
	// directo vía BFF → worker.
	mux.Handle("/agent/runtimes", requireRequire(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleRuntimes(w, r)
	})))
}

// --- handlers ---

type createSessionRequest struct {
	Title string `json:"title"`
	CWD   string `json:"cwd"`
	Model string `json:"model"`
}

type messageRequest struct {
	Message string `json:"message"`
	Action  string `json:"action,omitempty"`
	TurnID  string `json:"turnId,omitempty"`
}

func handleListSessions(w http.ResponseWriter, r *http.Request, mgr agentapp.AgentService) {
	sessions, err := mgr.List(r.Context())
	if err != nil {
		status, detail := mapSessionError(err)
		http.Error(w, detail, status)
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func handleCreateSession(w http.ResponseWriter, r *http.Request, mgr agentapp.AgentService) {
	body, err := readJSON[createSessionRequest](r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	session, err := mgr.Create(r.Context(), agentapp.CreateSessionInput{
		Title: body.Title,
		CWD:   body.CWD,
		Model: body.Model,
	})
	if err != nil {
		status, detail := mapSessionError(err)
		http.Error(w, detail, status)
		return
	}
	w.Header().Set("Location", "/agent?session="+session.ID)
	_ = writeJSON(w, http.StatusCreated, map[string]any{"session": session})
}

func handleGetSession(w http.ResponseWriter, r *http.Request, mgr agentapp.AgentService, id string) {
	session, err := mgr.Get(r.Context(), id)
	if err != nil {
		status, detail := mapSessionError(err)
		http.Error(w, detail, status)
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{"session": session})
}

func handleHistory(w http.ResponseWriter, r *http.Request, mgr agentapp.AgentService, id string) {
	if _, err := mgr.Get(r.Context(), id); err != nil {
		status, detail := mapSessionError(err)
		http.Error(w, detail, status)
		return
	}
	history, err := agentapp.LoadConversationHistory(
		id,
		agentapp.ParseHistoryBefore(r.URL.Query().Get("before")),
		agentapp.ParseHistoryLimit(r.URL.Query().Get("limit"), 30),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = writeJSON(w, http.StatusOK, history)
}

func handlePrompt(w http.ResponseWriter, r *http.Request, mgr agentapp.AgentService, id string) {
	body, err := readJSON[messageRequest](r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	body.Message = strings.TrimSpace(body.Message)
	body.Action = strings.TrimSpace(body.Action)
	body.TurnID = strings.TrimSpace(body.TurnID)
	if body.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}
	if err := mgr.PromptRequest(r.Context(), id, agentapp.PromptInput{
		Message: body.Message,
		Action:  agentapp.PromptAction(body.Action),
		TurnID:  body.TurnID,
	}); err != nil {
		status, detail := mapSessionError(err)
		http.Error(w, detail, status)
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleSteer(w http.ResponseWriter, r *http.Request, mgr agentapp.AgentService, id string) {
	body, err := readJSON[messageRequest](r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	message := strings.TrimSpace(body.Message)
	if message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}
	if err := mgr.Steer(r.Context(), id, message); err != nil {
		status, detail := mapSessionError(err)
		http.Error(w, detail, status)
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleAbort(w http.ResponseWriter, r *http.Request, mgr agentapp.AgentService, id string) {
	if err := mgr.Abort(r.Context(), id); err != nil {
		status, detail := mapSessionError(err)
		http.Error(w, detail, status)
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
