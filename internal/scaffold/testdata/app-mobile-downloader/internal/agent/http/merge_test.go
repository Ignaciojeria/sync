package agent

import (
	"context"
	"encoding/json"
	agentapp "lastmile-agents/internal/agent/application"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mergeServiceStub struct {
	result agentapp.MergeResult
	err    error
}

func (s mergeServiceStub) List(context.Context) ([]agentapp.Session, error) { return nil, nil }
func (s mergeServiceStub) Create(context.Context, agentapp.CreateSessionInput) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s mergeServiceStub) Get(context.Context, string) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s mergeServiceStub) Ensure(context.Context, string) error         { return nil }
func (s mergeServiceStub) Prompt(context.Context, string, string) error { return nil }
func (s mergeServiceStub) PromptRequest(context.Context, string, agentapp.PromptInput) error {
	return nil
}
func (s mergeServiceStub) Steer(context.Context, string, string) error { return nil }
func (s mergeServiceStub) Regenerate(context.Context, string) error   { return nil }
func (s mergeServiceStub) Abort(context.Context, string) error         { return nil }
func (s mergeServiceStub) Subscribe(context.Context, string) (<-chan agentapp.Event, func(), error) {
	return nil, func() {}, nil
}
func (s mergeServiceStub) RegisterPreview(context.Context, string, agentapp.RegisterPreviewInput) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s mergeServiceStub) ClearPreview(context.Context, string) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s mergeServiceStub) ApplyPreview(context.Context, string) (agentapp.ApplyResult, error) {
	return agentapp.ApplyResult{}, nil
}
func (s mergeServiceStub) MergePreview(context.Context, string) (agentapp.MergeResult, error) {
	return s.result, s.err
}
func (s mergeServiceStub) Delete(context.Context, string) error { return nil }
func (s mergeServiceStub) Close() error                         { return nil }

func TestMergePreviewHandler_OK(t *testing.T) {
	h := mergePreview(mergeServiceStub{result: agentapp.MergeResult{BaseBranch: "main", PreviewBranch: "agent/s1", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodPost, "/agent/sessions/s1/merge", nil)
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

func TestMergePreviewHandler_MapsConflictTo409(t *testing.T) {
	h := mergePreview(mergeServiceStub{err: agentapp.ErrPreviewMergeConflict})
	req := httptest.NewRequest(http.MethodPost, "/agent/sessions/s1/merge", nil)
	req.SetPathValue("id", "s1")
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
	}
}
