package agent

import (
	"encoding/json"
	agentapp "fixtests1/internal/agent/application"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetPreviewContext_InactiveOutsidePreview(t *testing.T) {
	h := getPreviewContext(pageServiceStub{})
	req := httptest.NewRequest(http.MethodGet, "/agent/preview-context", nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if got := body["active"]; got != false {
		t.Fatalf("active = %v, want false", got)
	}
}

func TestGetPreviewContext_ActiveInsidePreview(t *testing.T) {
	h := getPreviewContext(pageServiceStub{sessions: map[string]agentapp.Session{"p1": {ID: "p1", Branch: "agent/p1", BaseBranch: "main", WorkspacePath: "/preview", SourcePath: "/source"}}})
	req := httptest.NewRequest(http.MethodGet, "/agent/preview-context", nil)
	req.Header.Set("X-Forwarded-Prefix", "/agent/sessions/p1/preview/")
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if got := body["active"]; got != true {
		t.Fatalf("active = %v, want true", got)
	}
	if got := body["applicable"]; got != true {
		t.Fatalf("applicable = %v, want true", got)
	}
}
