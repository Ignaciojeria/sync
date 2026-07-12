package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"

	agentapp "testboi1/pkg/agent/application"

	"github.com/go-fuego/fuego"
)

func TestGetPreviewContext_InactiveOutsidePreview(t *testing.T) {
	h := getPreviewContext(pageServiceStub{})
	req := httptest.NewRequest(http.MethodGet, "/agent/preview-context", nil)
	ctx := fuego.NewMockContextNoBody()
	ctx.SetRequest(req)
	result, err := h(ctx)
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}
	body := result.(map[string]any)
	if got := body["active"]; got != false {
		t.Fatalf("active = %v, want false", got)
	}
}

func TestGetPreviewContext_ActiveInsidePreview(t *testing.T) {
	h := getPreviewContext(pageServiceStub{sessions: map[string]agentapp.Session{"p1": {ID: "p1", Branch: "agent/p1", BaseBranch: "main", WorkspacePath: "/preview", SourcePath: "/source"}}})
	req := httptest.NewRequest(http.MethodGet, "/agent/preview-context", nil)
	req.Header.Set("X-Forwarded-Prefix", "/agent/sessions/p1/preview/")
	ctx := fuego.NewMockContextNoBody()
	ctx.SetRequest(req)
	result, err := h(ctx)
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}
	body := result.(map[string]any)
	if got := body["active"]; got != true {
		t.Fatalf("active = %v, want true", got)
	}
	if got := body["applicable"]; got != true {
		t.Fatalf("applicable = %v, want true", got)
	}
}
