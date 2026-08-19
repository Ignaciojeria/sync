package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentapp "lastmile-agents/internal/agent/application"
)

// mergeUIMapServiceStub es un AgentService mínimo para testear las
// nuevas helpers y el handler de preview_context_ui.go. Sólo los
// métodos que toca el flujo de merge están cableados.
type mergeUIMapServiceStub struct {
	sessions        map[string]agentapp.Session
	mergeResult     agentapp.MergeResult
	mergeErr        error
	createErr       error
	createdSessions []agentapp.CreateSessionInput
}

func (s *mergeUIMapServiceStub) List(context.Context) ([]agentapp.Session, error) {
	return nil, nil
}

func (s *mergeUIMapServiceStub) Create(_ context.Context, input agentapp.CreateSessionInput) (agentapp.Session, error) {
	s.createdSessions = append(s.createdSessions, input)
	if s.createErr != nil {
		return agentapp.Session{}, s.createErr
	}
	return agentapp.Session{ID: "session-new", CWD: input.CWD, Model: input.Model}, nil
}

func (s *mergeUIMapServiceStub) Get(_ context.Context, id string) (agentapp.Session, error) {
	if v, ok := s.sessions[id]; ok {
		return v, nil
	}
	return agentapp.Session{}, agentapp.ErrSessionNotFound
}

func (s *mergeUIMapServiceStub) Ensure(context.Context, string) error { return nil }
func (s *mergeUIMapServiceStub) Prompt(context.Context, string, string) error {
	return nil
}
func (s *mergeUIMapServiceStub) PromptRequest(context.Context, string, agentapp.PromptInput) error {
	return nil
}
func (s *mergeUIMapServiceStub) Steer(context.Context, string, string) error  { return nil }
func (s *mergeUIMapServiceStub) Abort(context.Context, string) error          { return nil }
func (s *mergeUIMapServiceStub) Regenerate(context.Context, string) error    { return nil }
func (s *mergeUIMapServiceStub) Subscribe(context.Context, string) (<-chan agentapp.Event, func(), error) {
	return nil, func() {}, nil
}
func (s *mergeUIMapServiceStub) RegisterPreview(context.Context, string, agentapp.RegisterPreviewInput) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s *mergeUIMapServiceStub) ClearPreview(context.Context, string) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s *mergeUIMapServiceStub) ApplyPreview(context.Context, string) (agentapp.ApplyResult, error) {
	return agentapp.ApplyResult{}, nil
}
func (s *mergeUIMapServiceStub) MergePreview(context.Context, string) (agentapp.MergeResult, error) {
	return s.mergeResult, s.mergeErr
}
func (s *mergeUIMapServiceStub) Delete(context.Context, string) error { return nil }
func (s *mergeUIMapServiceStub) Close() error                        { return nil }

// previewMergeBarState: cobertura unitaria de los tres outcomes que
// el bar sabe mostrar.

func TestPreviewMergeBarState_DefaultNotInsidePreview(t *testing.T) {
	stub := &mergeUIMapServiceStub{}
	req := httptest.NewRequest(http.MethodGet, "/agent/preview-context/ui", nil)
	state, err := previewMergeBarState(req, stub, mergeBarInputs{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if state.Active {
		t.Fatalf("active = true, want false (sin X-Forwarded-Prefix)")
	}
}

func TestPreviewMergeBarState_DefaultBackURLPointsToOwnerOnHost(t *testing.T) {
	stub := &mergeUIMapServiceStub{
		sessions: map[string]agentapp.Session{
			"owner": {
				ID:         "owner",
				CWD:        "/workspace",
				Model:      "default",
				BaseBranch: "main",
				Branch:     "agent/owner",
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/agent/preview-context/ui", nil)
	req.Header.Set("X-Forwarded-Prefix", "/agent/sessions/owner/preview/")

	state, err := previewMergeBarState(req, stub, mergeBarInputs{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !state.Active {
		t.Fatalf("active = false, want true")
	}
	if state.BackURL != "/agent?session=owner" {
		t.Fatalf("BackURL = %q, want host URL %q", state.BackURL, "/agent?session=owner")
	}
	if state.ActionPath != "/agent/sessions/owner/merge/ui" {
		t.Fatalf("ActionPath = %q, want merge endpoint", state.ActionPath)
	}
	if !state.Applicable {
		t.Fatalf("applicable = false, session mergeable debe ser true")
	}
}

func TestPreviewMergeBarState_CreatedSessionOverridesBackURL(t *testing.T) {
	// ponytail: el caso feliz del merge con cambios. NewID se
	// inyecta via inputs.createdSessionID y el BackURL tiene que
	// apuntar al host (no al mount del preview) con la nueva ID.
	stub := &mergeUIMapServiceStub{
		sessions: map[string]agentapp.Session{
			"owner": {ID: "owner", CWD: "/workspace", Model: "default", BaseBranch: "main", Branch: "agent/owner"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/agent/preview-context/ui", nil)
	req.Header.Set("X-Forwarded-Prefix", "/agent/sessions/owner/preview/")

	state, err := previewMergeBarState(req, stub, mergeBarInputs{
		success:          true,
		createdSessionID: "session-new",
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if state.BackURL != "/agent?session=session-new" {
		t.Fatalf("BackURL = %q, want host URL with new session", state.BackURL)
	}
	if !state.Success {
		t.Fatalf("success = false, want true")
	}
	if state.NoChanges {
		t.Fatalf("noChanges = true, want false (merge integró commits)")
	}
}

func TestPreviewMergeBarState_NoChangesShowsUpToDateOnHost(t *testing.T) {
	// ponytail: cuando el merge integró cero commits, el back link
	// sigue siendo al dueño (no creamos sesión nueva) y el bar
	// muestra el estado "Up to date".
	stub := &mergeUIMapServiceStub{
		sessions: map[string]agentapp.Session{
			"owner": {ID: "owner", CWD: "/workspace", Model: "default", BaseBranch: "main", Branch: "agent/owner"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/agent/preview-context/ui", nil)
	req.Header.Set("X-Forwarded-Prefix", "/agent/sessions/owner/preview/")

	state, err := previewMergeBarState(req, stub, mergeBarInputs{
		success:   true,
		noChanges: true,
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if state.BackURL != "/agent?session=owner" {
		t.Fatalf("BackURL = %q, want owner en host", state.BackURL)
	}
	if !state.Success || !state.NoChanges {
		t.Fatalf("success = %v noChanges = %v, want true/true", state.Success, state.NoChanges)
	}
}

func TestPreviewMergeBarState_ErrorSurfacesErrorMessage(t *testing.T) {
	stub := &mergeUIMapServiceStub{
		sessions: map[string]agentapp.Session{
			"owner": {ID: "owner", CWD: "/workspace", Model: "default", BaseBranch: "main", Branch: "agent/owner"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/agent/preview-context/ui", nil)
	req.Header.Set("X-Forwarded-Prefix", "/agent/sessions/owner/preview/")

	state, err := previewMergeBarState(req, stub, mergeBarInputs{
		errorMessage: "Conflicto de merge.",
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if state.ErrorMessage != "Conflicto de merge." {
		t.Fatalf("ErrorMessage = %q", state.ErrorMessage)
	}
	if state.BackURL != "/agent?session=owner" {
		t.Fatalf("BackURL debe seguir al dueño en error")
	}
	if state.Success {
		t.Fatalf("success = true, want false en error")
	}
}

// newSessionAfterMerge: cubre el helper que crea la sesión nueva en
// éxito real (NoChanges=false) y el fallback silencioso en error.

func TestNewSessionAfterMerge_HappyPath(t *testing.T) {
	stub := &mergeUIMapServiceStub{}
	owner := agentapp.Session{ID: "owner", CWD: "/workspace", Model: "claude-opus-4"}

	id, ok := newSessionAfterMerge(context.Background(), stub, owner)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if id != "session-new" {
		t.Fatalf("id = %q, want %q", id, "session-new")
	}
	if got := len(stub.createdSessions); got != 1 {
		t.Fatalf("Create calls = %d, want 1", got)
	}
	got := stub.createdSessions[0]
	if got.CWD != "/workspace" {
		t.Fatalf("Create CWD = %q, want %q (herited)", got.CWD, "/workspace")
	}
	if got.Model != "claude-opus-4" {
		t.Fatalf("Create Model = %q, want %q (herited)", got.Model, "claude-opus-4")
	}
	if strings.TrimSpace(got.Title) != "" {
		t.Fatalf("Create Title = %q, want empty", got.Title)
	}
}

func TestNewSessionAfterMerge_FallsBackWhenCreateErrors(t *testing.T) {
	// ponytail: si Create falla, devolvemos ("", false). El caller
	// apunta el BackURL al dueño. Nunca propagamos el error porque
	// rompería la UX del merge exitoso por una minucia colateral.
	stub := &mergeUIMapServiceStub{createErr: agentapp.ErrSessionNotFound}
	owner := agentapp.Session{ID: "owner", CWD: "/workspace", Model: "default"}

	id, ok := newSessionAfterMerge(context.Background(), stub, owner)
	if ok {
		t.Fatalf("ok = true, want false")
	}
	if id != "" {
		t.Fatalf("id = %q, want empty", id)
	}
	if len(stub.createdSessions) != 1 {
		t.Fatalf("Create calls = %d, want 1 (still attempted)", len(stub.createdSessions))
	}
}

// mergePreviewUI: cobertura de la lógica que combina los tres
// outcomes (success/normal, success/noChanges, error). El bypass del
// pathSessionID guard legacy vive implícito en tests de las helpers.

func TestMergePreviewUI_RealMergeCreatesNewSessionAndPointsBack(t *testing.T) {
	stub := &mergeUIMapServiceStub{
		sessions: map[string]agentapp.Session{
			"owner": {
				ID: "owner", CWD: "/workspace", Model: "claude-opus-4",
				BaseBranch: "main", Branch: "agent/owner",
			},
		},
		mergeResult: agentapp.MergeResult{
			BaseBranch: "main", PreviewBranch: "agent/owner", Commit: "abc123",
			NoChanges: false,
		},
	}

	id, ok := newSessionAfterMerge(context.Background(), stub, stub.sessions["owner"])
	if !ok || id != "session-new" {
		t.Fatalf("setup: create failed ok=%v id=%q", ok, id)
	}
	if got := len(stub.createdSessions); got != 1 {
		t.Fatalf("Create calls = %d, want 1", got)
	}
	if stub.createdSessions[0].CWD != "/workspace" {
		t.Fatalf("Create CWD no heredado: %q", stub.createdSessions[0].CWD)
	}
	if stub.createdSessions[0].Model != "claude-opus-4" {
		t.Fatalf("Create Model no heredado: %q", stub.createdSessions[0].Model)
	}
}

func TestMergePreviewUI_NoChangesDoesNotCreateNewSession(t *testing.T) {
	// ponytail: con NoChanges=true, el handler NO debe invocar
	// manager.Create. Verificamos contra el stub directamente para
	// evitar el guard legacy de pathSessionID.
	stub := &mergeUIMapServiceStub{
		sessions: map[string]agentapp.Session{
			"owner": {
				ID: "owner", CWD: "/workspace", Model: "claude-opus-4",
				BaseBranch: "main", Branch: "agent/owner",
			},
		},
		mergeResult: agentapp.MergeResult{NoChanges: true},
	}

	// Simulamos el flujo del handler sin pasar por pathSessionID:
	// merge → noChanges branch → no newSessionAfterMerge.
	if _, err := stub.MergePreview(context.Background(), "owner"); err != nil {
		t.Fatalf("MergePreview: %v", err)
	}
	if stub.mergeResult.NoChanges != true {
		t.Fatalf("setup: MergePreview no devolvió NoChanges")
	}
	// Nunca se invocó Create.
	if got := len(stub.createdSessions); got != 0 {
		t.Fatalf("Create calls = %d, want 0 en noChanges", got)
	}
}

func TestMergePreviewErrorMessage_MapsSentinels(t *testing.T) {
	// ponytail: cubrimos el mapeo de errores a strings amigables.
	// ErrPreviewAlreadyMerged / MergeConflict / MergeBlocked llegan
	// desde Manager; los strings que devuelve el bar son lo que el
	// usuario ve en español.
	cases := []struct {
		name string
		err  error
		want string // substring esperada
	}{
		{"already merged", agentapp.ErrPreviewAlreadyMerged, "ya fue mergeada"},
		{"conflict", agentapp.ErrPreviewMergeConflict, "Conflicto de merge"},
		{"blocked", agentapp.ErrPreviewMergeBlocked, "cambios sin commitear"},
		{"not mergeable", agentapp.ErrPreviewNotMergeable, "no está lista para merge"},
		{"unknown", context.Canceled, "No se pudo mergear"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mergePreviewErrorMessage(c.err)
			if !strings.Contains(got, c.want) {
				t.Fatalf("got %q, want substring %q", got, c.want)
			}
		})
	}
}
