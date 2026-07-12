package layout

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestPreviewMergeBar_DefaultShowsApplyButton(t *testing.T) {
	state := PreviewMergeBarState{
		Active:        true,
		SessionID:     "agent-x",
		BaseBranch:    "main",
		PreviewBranch: "agent/agent-x",
		Applicable:    true,
		ActionPath:    "/agent/sessions/agent-x/apply/ui",
	}
	body := renderPreviewMergeBar(t, state)
	mustContain(t, body, "Apply preview")
	mustContain(t, body, "alert-info")
	mustContain(t, body, `hx-post="/agent/sessions/agent-x/apply/ui"`)
	if strings.Contains(body, "Volver al inicio") {
		t.Fatalf("did not expect success link in default state, got: %s", body)
	}
	if strings.Contains(body, "alert-success") {
		t.Fatalf("did not expect success tone in default state, got: %s", body)
	}
}

func TestPreviewMergeBar_SuccessShowsAppliedBadgeAndHomeLink(t *testing.T) {
	state := PreviewMergeBarState{
		Active:        true,
		SessionID:     "agent-x",
		BaseBranch:    "main",
		PreviewBranch: "agent/agent-x",
		Applicable:    true,
		ActionPath:    "/agent/sessions/agent-x/apply/ui",
		Success:       true,
	}
	body := renderPreviewMergeBar(t, state)
	mustContain(t, body, "Volver al inicio")
	mustContain(t, body, "alert-success")
	mustContain(t, body, "badge-success")
	mustContain(t, body, "Applied")
	mustContain(t, body, "Cambios aplicados a main.")
	if strings.Contains(body, "Apply preview") {
		t.Fatalf("did not expect apply button in success state, got: %s", body)
	}
	if strings.Contains(body, `hx-post`) {
		t.Fatalf("did not expect hx-post in success state, got: %s", body)
	}
}

func TestPreviewMergeBar_ErrorTakesPrecedenceOverSuccess(t *testing.T) {
	state := PreviewMergeBarState{
		Active:        true,
		SessionID:     "agent-x",
		BaseBranch:    "main",
		Applicable:    true,
		ActionPath:    "/agent/sessions/agent-x/apply/ui",
		Success:       true,
		ErrorMessage:  "No se pudo aplicar esta preview.",
	}
	body := renderPreviewMergeBar(t, state)
	mustContain(t, body, "alert-error")
	mustContain(t, body, "No se pudo aplicar esta preview.")
	mustContain(t, body, "Apply preview")
	if strings.Contains(body, "alert-success") {
		t.Fatalf("did not expect success tone when error present, got: %s", body)
	}
	if strings.Contains(body, "Volver al inicio") {
		t.Fatalf("did not expect home link when error present, got: %s", body)
	}
}

func TestPreviewMergeBar_NotApplicableShowsDisabledButton(t *testing.T) {
	state := PreviewMergeBarState{
		Active:     true,
		SessionID:  "agent-x",
		Applicable: false,
	}
	body := renderPreviewMergeBar(t, state)
	mustContain(t, body, "Apply unavailable")
	mustContain(t, body, "btn-disabled")
	if strings.Contains(body, "Volver al inicio") {
		t.Fatalf("did not expect home link when not applicable, got: %s", body)
	}
}

func TestPreviewMergeBar_InactiveEmptiesContainer(t *testing.T) {
	state := PreviewMergeBarState{Active: false}
	body := renderPreviewMergeBar(t, state)
	mustContain(t, body, `<div id="global-preview-merge-bar"></div>`)
	if strings.Contains(body, "Apply preview") || strings.Contains(body, "Volver al inicio") {
		t.Fatalf("inactive bar should be empty, got: %s", body)
	}
}

func renderPreviewMergeBar(t *testing.T, state PreviewMergeBarState) string {
	t.Helper()
	var buf bytes.Buffer
	if err := PreviewMergeBar(state).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render PreviewMergeBar: %v", err)
	}
	return buf.String()
}

func mustContain(t *testing.T, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("expected body to contain %q, got: %s", want, body)
	}
}
