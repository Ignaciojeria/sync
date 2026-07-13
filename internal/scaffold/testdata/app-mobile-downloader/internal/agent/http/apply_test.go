package agent

import (
	"context"
	"encoding/json"
	agentapp "fixtests1/internal/agent/application"
	"net/http"
	"net/http/httptest"
	"testing"
)

type applyServiceStub struct {
	result agentapp.ApplyResult
	err    error
}

func (s applyServiceStub) List(context.Context) ([]agentapp.Session, error) { return nil, nil }
func (s applyServiceStub) Create(context.Context, agentapp.CreateSessionInput) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s applyServiceStub) Get(context.Context, string) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s applyServiceStub) Ensure(context.Context, string) error         { return nil }
func (s applyServiceStub) Prompt(context.Context, string, string) error { return nil }
func (s applyServiceStub) PromptRequest(context.Context, string, agentapp.PromptInput) error {
	return nil
}
func (s applyServiceStub) Steer(context.Context, string, string) error { return nil }
func (s applyServiceStub) Abort(context.Context, string) error         { return nil }
func (s applyServiceStub) Subscribe(context.Context, string) (<-chan agentapp.Event, func(), error) {
	return nil, func() {}, nil
}
func (s applyServiceStub) RegisterPreview(context.Context, string, agentapp.RegisterPreviewInput) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s applyServiceStub) ClearPreview(context.Context, string) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s applyServiceStub) ApplyPreview(context.Context, string) (agentapp.ApplyResult, error) {
	return s.result, s.err
}
func (s applyServiceStub) MergePreview(context.Context, string) (agentapp.MergeResult, error) {
	return agentapp.MergeResult{}, nil
}
func (s applyServiceStub) Delete(context.Context, string) error { return nil }
func (s applyServiceStub) Close() error                         { return nil }

func TestApplyPreviewHandler_OK(t *testing.T) {
	h := applyPreview(applyServiceStub{result: agentapp.ApplyResult{SourcePath: "/src", PreviewPath: "/preview"}})
	req := httptest.NewRequest(http.MethodPost, "/agent/sessions/s1/apply", nil)
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
