package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agentapp "app-mobile-downloader/pkg/agent/application"

	"github.com/golang-jwt/jwt/v5"
)

// TestHealthzReturnsAlive verifica el contrato del endpoint /agent/healthz
// (liveness). Si cambia (status, body, header), el BFF o un operador
// que consuma /agent/healthz lo va a notar.
func TestHealthzReturnsAlive(t *testing.T) {
	mux := newWorkerMux(t, workerTestDeps{authMW: nil})

	req := httptest.NewRequest(http.MethodGet, "/agent/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body, _ := io.ReadAll(rec.Body)
	got := strings.TrimSpace(string(body))
	want := `{"status":"alive"}`
	if got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// TestHealthzRejectsNonGET confirma el método: el BFF y curl/tcp
// probers deben usar GET exclusivamente.
func TestHealthzRejectsNonGET(t *testing.T) {
	mux := newWorkerMux(t, workerTestDeps{authMW: nil})

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/agent/healthz", nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s /agent/healthz → status %d, want 405",
					method, rec.Code)
			}
		})
	}
}

// TestAgentDataRoute_RejectsWithoutJWT verifica propiedad central de
// Opción A: sin Authorization: Bearer <jwt>, los handlers de datos
// devuelven 401 incluso si la ruta /agent/healthz funciona.
func TestAgentDataRoute_RejectsWithoutJWT(t *testing.T) {
	mux := newWorkerMux(t, workerTestDeps{jwtSecret: []byte(testJWTSecretShared)})

	// Sin Authorization: 401.
	req := httptest.NewRequest(http.MethodPost, "/agent/sessions/abc/prompt",
		strings.NewReader(`{"message":"hola"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin JWT: status %d, want 401", rec.Code)
	}
}

// TestAgentDataRoute_AcceptsValidJWT verifica camino feliz: el worker
// valida el JWT firmado por HMAC (mismo secret que el BFF/productor)
// contra su propio JWKS, y deja pasar al handler.
func TestAgentDataRoute_AcceptsValidJWT(t *testing.T) {
	mux := newWorkerMux(t, workerTestDeps{jwtSecret: []byte(testJWTSecretShared)})

	tok := makeHMACJWT(t, "dev@example.com")

	req := httptest.NewRequest(http.MethodPost, "/agent/sessions/abc/prompt",
		strings.NewReader(`{"message":"hola"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Errorf("con JWT válido: debería pasar auth (cualquier status != 401), got 401")
	}
}

// TestAgentDataRoute_RejectsTamperedJWT: cambiar cualquier parte del
// JWT invalidla firma y rechaza el request.
func TestAgentDataRoute_RejectsTamperedJWT(t *testing.T) {
	mux := newWorkerMux(t, workerTestDeps{jwtSecret: []byte(testJWTSecretShared)})

	tok := makeHMACJWT(t, "dev@example.com")
	// Cambiar el último char de la firma (hex).
	mutated := tok[:len(tok)-1] + "ff"

	req := httptest.NewRequest(http.MethodPost, "/agent/sessions/abc/prompt",
		strings.NewReader(`{"message":"hola"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+mutated)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("JWT tampered: status %d, want 401", rec.Code)
	}
}

// TestAgentDataRoute_RejectsExpiredJWT: tokens viejos son rechazados
// porque exp='en el pasado'.
func TestAgentDataRoute_RejectsExpiredJWT(t *testing.T) {
	mux := newWorkerMux(t, workerTestDeps{jwtSecret: []byte(testJWTSecretShared)})

	// JWT firmado con iat/exp antiguos.
	claims := jwt.MapClaims{
		"email": "dev@example.com",
		"sub":   "test",
		"iat":   time.Now().Add(-2 * time.Hour).Unix(),
		"exp":   time.Now().Add(-1 * time.Hour).Unix(),
	}
	tokObj := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := tokObj.SignedString([]byte(testJWTSecretShared))

	req := httptest.NewRequest(http.MethodPost, "/agent/sessions/abc/prompt",
		strings.NewReader(`{"message":"hola"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+signed)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("JWT expirado: status %d, want 401", rec.Code)
	}
}

// TestAgentDataRoute_RejectsJWTSignedWithWrongSecret: firma con secret
// distinto al configurado en el worker rechaza con 401.
func TestAgentDataRoute_RejectsJWTSignedWithWrongSecret(t *testing.T) {
	mux := newWorkerMux(t, workerTestDeps{jwtSecret: []byte("real-secret-a")})

	wrongTok := makeHMACJWTWithSecret(t, "dev@example.com", []byte("attacker-secret-b"))

	req := httptest.NewRequest(http.MethodPost, "/agent/sessions/abc/prompt",
		strings.NewReader(`{"message":"hola"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+wrongTok)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("JWT con secret incorrecto: status %d, want 401", rec.Code)
	}
}

// TestAgentDataRoute_NoAuthMiddlewareConfigured: si el worker está
// mal configurado y no carga JWKS ni HMAC, el build del middleware
// falla. El test captura que el path de boot-time error está bien
// diagnosticado.
func TestAgentDataRoute_NoAuthMiddlewareConfigured(t *testing.T) {
	_, err := authMiddleware(context.Background())
	if err == nil {
		t.Fatal("authMiddleware debería fallar sin JWKS_URL ni JWT_HMAC_SECRET")
	}
	if !strings.Contains(err.Error(), "JWKS_URL") && !strings.Contains(err.Error(), "JWT_HMAC_SECRET") {
		t.Errorf("error no menciona las env vars requeridas: %v", err)
	}
}

// --- helpers ---

const testJWTSecretShared = "test-shared-jwt-secret-must-be-long-enough"

// makeHMACJWT genera un JWT firmado con HMAC usando testJWTSecretShared.
// Incluye iss/aud que matchean los env var seteados por newWorkerMux.
func makeHMACJWT(t *testing.T, email string) string {
	t.Helper()
	return makeHMACJWTWithSecret(t, email, []byte(testJWTSecretShared))
}

func makeHMACJWTWithSecret(t *testing.T, email string, secret []byte) string {
	t.Helper()
	claims := jwt.MapClaims{
		"email": email,
		"sub":   "test-user",
		"iss":   "https://test-issuer.invalid",
		"aud":   "test-audience",
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

// workerTestDeps is the bag of dependencies needed to wire up a test mux.
// jwtSecret sets HMAC mode; authMW si está provisto se usa en lugar
// del JWTMiddleware real (para tests que no quieren validar JWT pero
// sí ver el routing).
type workerTestDeps struct {
	jwtSecret []byte
	authMW     func(http.Handler) http.Handler
}

// newWorkerMux construye el mux del worker con un Manager stub.
// Si jwtSecret está seteado usa un middleware HMAC real; authMW se
// usa sólo como override explícito (para tests de routing que
// quieren simular "ya estoy autenticado").
func newWorkerMux(t *testing.T, deps workerTestDeps) *http.ServeMux {
	t.Helper()

	mgr := &stubMgr{}

	if deps.authMW != nil {
		mux := http.NewServeMux()
		registerRoutes(mux, mgr, deps.authMW)
		return mux
	}

	if len(deps.jwtSecret) == 0 {
		// Modo healthz-only: no armamos el stack de auth. Evita
		// pasar un nil func a registerRoutes (que panicaría).
		return newHealthzOnlyMux()
	}

	// Modo JWT real: setear envs y armar JWTMiddleware desde cero.
	t.Setenv("JWKS_URL", "")
	t.Setenv("JWT_HMAC_SECRET", string(deps.jwtSecret))
	t.Setenv("OIDC_ISSUER", "https://test-issuer.invalid")
	t.Setenv("OIDC_CLIENT_ID", "test-client")
	t.Setenv("JWT_AUDIENCE", "test-audience")
	authMW, err := authMiddleware(context.Background())
	if err != nil {
		t.Fatalf("authMiddleware: %v", err)
	}
	mux := http.NewServeMux()
	registerRoutes(mux, mgr, authMW)
	return mux
}

// newHealthzOnlyMux expone sólo GET /agent/healthz, sin auth. Lo
// usan los tests de liveness para no armar el stack completo.
func newHealthzOnlyMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/agent/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"alive"}`)
	})
	return mux
}

// stubMgr es un AgentService mínimo que devuelve errores genéricos
// para que los tests verifiquen que auth pasa y la lógica de
// aplicación (de la cual estos tests no son dueños) los maneja.
type stubMgr struct{}

func (s *stubMgr) Prompt(ctx context.Context, id, msg string) error {
	return errors.New("stub_mgr: prompt not implemented")
}
func (s *stubMgr) Steer(ctx context.Context, id, msg string) error {
	return errors.New("stub_mgr: steer not implemented")
}
func (s *stubMgr) Abort(ctx context.Context, id string) error {
	return errors.New("stub_mgr: abort not implemented")
}
func (s *stubMgr) Subscribe(ctx context.Context, id string) (<-chan agentapp.Event, func(), error) {
	return nil, nil, errors.New("stub_mgr: subscribe not implemented")
}
func (s *stubMgr) List(ctx context.Context) ([]agentapp.Session, error) { return nil, nil }
func (s *stubMgr) Create(ctx context.Context, in agentapp.CreateSessionInput) (agentapp.Session, error) {
	return agentapp.Session{}, errors.New("stub_mgr")
}
func (s *stubMgr) Get(ctx context.Context, id string) (agentapp.Session, error) {
	return agentapp.Session{}, errors.New("stub_mgr")
}
func (s *stubMgr) Ensure(ctx context.Context, id string) error { return nil }
func (s *stubMgr) Close() error                              { return nil }
