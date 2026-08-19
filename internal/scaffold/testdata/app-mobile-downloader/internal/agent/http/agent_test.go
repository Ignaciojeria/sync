package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentapp "lastmile-agents/internal/agent/application"
	"lastmile-agents/internal/shared/server"
)

// newTestAgentServer levanta un server mínimo para tests de routing
// del agente. Usa nil en casi todo lo que no afecta el caso bajo
// prueba — el requireEditor del paquete auth termina siendo un
// no-op cuando allowEditor es true. Aquí testeamos que Register
// monta las rutas correctas (dashboard, static, data handlers),
// no la autenticación.
func newTestAgentServer(t *testing.T, manager agentapp.AgentService) *server.Server {
	t.Helper()
	s := server.NewServer()
	noopEditor := func(h http.Handler) http.Handler { return h }
	Register(s, manager, nil, OIDCRefreshConfig{}, noopEditor, nil)
	return s
}

// sessionsOnlyStub devuelve sólo las sesiones que se le piden. Es
// útil cuando el test sólo necesita ejercitar el redirect heuristic
// sin pasar por el resto del AgentService (que pageServiceStub ya
// implementa en page_test.go).
type sessionsOnlyStub struct {
	pageServiceStub
	sessions []agentapp.Session
}

func (s sessionsOnlyStub) List(context.Context) ([]agentapp.Session, error) {
	return s.sessions, nil
}

// TestRegister_MountsDashboardRoute verifica que GET /agent monta
// la shell del agente. Sin sesiones, el redirect heuristic manda
// a /agent/home (302). Si hubiera sesiones, mandaría a
// /agent?session=<id>.
func TestRegister_MountsDashboardRoute(t *testing.T) {
	stub := sessionsOnlyStub{sessions: []agentapp.Session{}}
	s := newTestAgentServer(t, stub)
	req := httptest.NewRequest(http.MethodGet, "/agent", nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("/agent debería redirigir cuando no hay sesiones, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasSuffix(loc, "/agent/home") {
		t.Fatalf("Location = %q, esperaba que termine en /agent/home", loc)
	}
}

// TestRegister_RedirectsToLatestSession verifica que la heurística
// de redirect funciona: si /agent no recibe ?session= y hay
// sesiones, redirige a la más reciente.
func TestRegister_RedirectsToLatestSession(t *testing.T) {
	stub := sessionsOnlyStub{sessions: []agentapp.Session{{ID: "s-latest"}, {ID: "s-older"}}}
	s := newTestAgentServer(t, stub)
	req := httptest.NewRequest(http.MethodGet, "/agent", nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("/agent debería redirigir cuando hay sesiones, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "session=s-latest") {
		t.Fatalf("Location = %q, esperaba que contenga session=s-latest", loc)
	}
}

// TestRegister_TrailingSlashRedirectsToCanonicalEntry verifica que
// /agent/ con query string redirige a /agent (canonical entry).
func TestRegister_TrailingSlashRedirectsToCanonicalEntry(t *testing.T) {
	s := newTestAgentServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/agent/?session=s1", nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("/agent/ debería canonizar a /agent, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/agent?session=s1" {
		t.Fatalf("Location = %q, esperaba /agent?session=s1", loc)
	}
}

// TestRegister_MountsStaticHandler verifica que el módulo JS del
// chat se sirve desde /agent/static/main.js. Sin este handler el
// browser recibe 404 al cargar el módulo y el chat no arranca.
func TestRegister_MountsStaticHandler(t *testing.T) {
	s := newTestAgentServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/agent/static/main.js", nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/agent/static/main.js debería responder 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("Content-Type = %q, esperaba application/javascript", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "v2-app") {
		t.Fatalf("body no parece ser el módulo main.js, got: %s", body)
	}
}

// TestRegister_StaticBlocksPathTraversal verifica que el handler de
// static no permite ../ para escapar del FS embebido.
func TestRegister_StaticBlocksPathTraversal(t *testing.T) {
	s := newTestAgentServer(t, nil)
	cases := []string{
		"/agent/static/../embed.go",
		"/agent/static/..%2fembed.go",
		"/agent/static/./../../etc/passwd",
	}
	for _, p := range cases {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)

		// 404 del mux o del http.FileServer. Lo importante es que
		// NO devuelva 200 con contenido fuera del FS del chat.
		if rec.Code == http.StatusOK && !strings.Contains(rec.Body.String(), "v2-") {
			t.Fatalf("%s escapó del sandbox (200 + body ajeno)", p)
		}
	}
}

// TestRegister_Static404ForMissing confirma que pedir un asset
// inexistente devuelve 404 sin listar el FS embebido.
func TestRegister_Static404ForMissing(t *testing.T) {
	s := newTestAgentServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/agent/static/does-not-exist.js", nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("/agent/static/does-not-exist.js debería devolver 404, got %d", rec.Code)
	}
}

// TestRegister_MountsDataHandlers verifica que los data handlers
// del agente están todos montados por la nueva Register (sin
// necesidad de un forwarder V1↔V2 que ya no existe). Algunos
// endpoints validan query params o requieren una sesión que
// exista; acá probamos que el handler responde (no 404 del mux),
// aunque la lógica de negocio pueda devolver 400/404.
//
// Nota: el endpoint /agent/sessions/{id}/events es un stream SSE
// que no retorna hasta que se cierra la conexión; lo testeamos
// indirectamente al final del test.
func TestRegister_MountsDataHandlers(t *testing.T) {
	stub := sessionsOnlyStub{
		pageServiceStub: pageServiceStub{
			sessions: map[string]agentapp.Session{
				"s1": {ID: "s1", Title: "t", CWD: "/tmp"},
			},
		},
	}
	s := newTestAgentServer(t, stub)

	cases := []struct {
		method     string
		path       string
		body       string
		wantStatus int
		// 200 si la lógica de negocio acepta el input, 4xx si
		// valida y rechaza (igual confirma que el handler está
		// montado: 404 sería del mux, no del handler).
	}{
		{http.MethodGet, "/agent/sessions", "", http.StatusOK},
		{http.MethodPost, "/agent/sessions", `{"title":"new"}`, http.StatusOK},
		{http.MethodGet, "/agent/sessions/s1", "", http.StatusOK},
		{http.MethodGet, "/agent/sessions/s1/history", "", http.StatusOK},
		{http.MethodPost, "/agent/sessions/s1/prompt", `{"message":"x"}`, http.StatusOK},
		{http.MethodPost, "/agent/sessions/s1/abort", `{}`, http.StatusOK},
		{http.MethodGet, "/agent/sessions/s1/worktree", "", http.StatusOK},
		// worktree/diff requiere ?file=... — 400 confirma que el
		// handler está montado.
		{http.MethodGet, "/agent/sessions/s1/worktree/diff", "", http.StatusBadRequest},
		// ponytail: regenerate sin user_prompt con Seq>0 → 400. El
		// sendPrompt handler escribe user_prompts con Seq=0 porque
		// MaterializeUserPrompt no asigna seq (vienen del POST antes
		// del flujo SSE/journal). El regenerate handler busca el
		// último user_prompt con Seq>0 para usar como punto de
		// corte (clearAfterSeq). Sin Seq>0, devuelve 400. Una mejora
		// futura sería aceptar Seq=0 también, pero eso cambia la
		// semántica del clearAfterSeq. Por ahora, 400 es lo correcto.
		{http.MethodPost, "/agent/sessions/s1/regenerate", `{}`, http.StatusBadRequest},
		// preview sin sesión creada vía la API real → 404 (la
		// sesión "s1" en el stub existe pero el manager real
		// mock no la trackea).
		{http.MethodPost, "/agent/sessions/s1/preview", `{"port":3000}`, http.StatusNotFound},
		{http.MethodGet, "/agent/preview-context", "", http.StatusOK},
	}
	for _, tc := range cases {
		var body *strings.Reader
		if tc.body != "" {
			body = strings.NewReader(tc.body)
		} else {
			body = strings.NewReader("")
		}
		req := httptest.NewRequest(tc.method, tc.path, body)
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)
		if rec.Code != tc.wantStatus {
			t.Errorf("%s %s: status = %d, want %d (body: %s)", tc.method, tc.path, rec.Code, tc.wantStatus, rec.Body.String())
		}
	}

	// Verificamos que el endpoint de SSE está montado. Lo testeamos
	// indirectamente: como el handler de SSE bloquea leyendo del
	// canal del manager, no podemos hacer un GET directo sin que
	// se cuelgue. En su lugar, comprobamos que la ruta devuelve
	// status != 404 cuando la ruteamos.
	t.Run("events_stream_is_mounted", func(t *testing.T) {
		// Verificamos via reflection: si la ruta no estuviera
		// registrada el mux devolvería 404. El handler SSE
		// mantiene la conexión abierta; un test que sólo prueba
		// el mount puede usar un cliente con deadline corta.
		ctx, cancel := context.WithTimeout(context.Background(), 100*1e6)
		defer cancel()
		req := httptest.NewRequest(http.MethodGet, "/agent/sessions/s1/events", nil).WithContext(ctx)
		req.SetPathValue("id", "s1")
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)
		// El SSE handler no escribe el status hasta que el canal
		// emite algo, así que con un context que vence rápido
		// obtenemos un body vacío o un error. Lo importante es
		// que NO obtengamos 404 (eso significaría que la ruta no
		// está montada).
		if rec.Code == http.StatusNotFound {
			t.Errorf("/agent/sessions/s1/events devolvió 404, esperaba handler montado")
		}
	})
}

// --- helpers --------------------------------------------------------

// TestIsSafeSessionID valida el whitelist del sessionID.
func TestIsSafeSessionID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"s1", true},
		{"agent-1784247-17", true},
		{"s_1", true},
		{"abc123", true},
		{"", false},
		{"a/b", false},
		{"a b", false},
		{"a.b", false},
		{"../escape", false},
		{strings.Repeat("a", 300), false}, // muy largo
	}
	for _, tc := range cases {
		got := isSafeSessionID(tc.in)
		if got != tc.want {
			t.Errorf("isSafeSessionID(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
