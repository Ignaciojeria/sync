package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	agentapp "testboi1/pkg/agent/application"

	"github.com/go-fuego/fuego"
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
	ctx := fuego.NewMockContextNoBody()
	ctx.PathParams["id"] = "s1"
	ctx.SetRequest(req)
	result, err := h(ctx)
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}
	body := result.(map[string]any)
	if got := body["ok"]; got != true {
		t.Fatalf("ok = %v, want true", got)
	}
}

func TestMergePreviewHandler_MapsConflictTo409(t *testing.T) {
	h := mergePreview(mergeServiceStub{err: agentapp.ErrPreviewMergeConflict})
	req := httptest.NewRequest(http.MethodPost, "/agent/sessions/s1/merge", nil)
	ctx := fuego.NewMockContextNoBody()
	ctx.PathParams["id"] = "s1"
	ctx.SetRequest(req)
	_, err := h(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
}
