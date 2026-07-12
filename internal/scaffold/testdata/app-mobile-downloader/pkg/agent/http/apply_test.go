package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	agentapp "testboi1/pkg/agent/application"

	"github.com/go-fuego/fuego"
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
