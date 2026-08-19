package agent

import (
	"context"
	"encoding/json"
	agentapp "lastmile-agents/internal/agent/application"
	"net/http"
	"net/http/httptest"
	"testing"
)

type abortServiceStub struct{ err error }

func (s abortServiceStub) List(context.Context) ([]agentapp.Session, error) { return nil, nil }
func (s abortServiceStub) Create(context.Context, agentapp.CreateSessionInput) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s abortServiceStub) Get(context.Context, string) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s abortServiceStub) Ensure(context.Context, string) error         { return nil }
func (s abortServiceStub) Prompt(context.Context, string, string) error { return nil }
func (s abortServiceStub) PromptRequest(context.Context, string, agentapp.PromptInput) error {
	return nil
}
func (s abortServiceStub) Steer(context.Context, string, string) error { return nil }
func (s abortServiceStub) Abort(context.Context, string) error         { return s.err }
func (s abortServiceStub) Regenerate(context.Context, string) error   { return s.err }
func (s abortServiceStub) Subscribe(context.Context, string) (<-chan agentapp.Event, func(), error) {
	return nil, func() {}, nil
}
func (s abortServiceStub) RegisterPreview(context.Context, string, agentapp.RegisterPreviewInput) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s abortServiceStub) ClearPreview(context.Context, string) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s abortServiceStub) ApplyPreview(context.Context, string) (agentapp.ApplyResult, error) {
	return agentapp.ApplyResult{}, nil
}
func (s abortServiceStub) MergePreview(context.Context, string) (agentapp.MergeResult, error) {
	return agentapp.MergeResult{}, nil
}
func (s abortServiceStub) Delete(context.Context, string) error { return nil }
func (s abortServiceStub) Close() error                         { return nil }

func TestAbortSession_OK(t *testing.T) {
	h := abortSession(abortServiceStub{})
	req := httptest.NewRequest(http.MethodPost, "/agent/sessions/s1/abort", nil)
	req.SetPathValue("id", "s1")
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if got := body["ok"]; got != true {
		t.Fatalf("ok = %v, want true", got)
	}
}

func TestAbortSession_MissingID(t *testing.T) {
	h := abortSession(abortServiceStub{})
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodPost, "/agent/sessions//abort", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestAbortSession_ManagerError(t *testing.T) {
	h := abortSession(abortServiceStub{err: agentapp.ErrSessionNotFound})
	req := httptest.NewRequest(http.MethodPost, "/agent/sessions/s1/abort", nil)
	req.SetPathValue("id", "s1")
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}
