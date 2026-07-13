package layout

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestPreviewMergeBar_ActiveShowsBadgeMessageAndBothButtons(t *testing.T) {
	state := PreviewMergeBarState{
		Active:        true,
		SessionID:     "agent-x",
		BackURL:       "/agent?session=agent-x",
		BaseBranch:    "main",
		PreviewBranch: "agent/agent-x",
		Applicable:    true,
		ActionPath:    "/agent/sessions/agent-x/apply/ui",
	}
	body := renderPreviewMergeBar(t, state)
	mustContain(t, body, "badge-info")
	mustContain(t, body, "Preview")
	mustContain(t, body, "Preview aislada")
	mustContain(t, body, `href="/agent?session=agent-x"`)
	mustContain(t, body, "Back to agent")
	mustContain(t, body, "Apply preview")
	mustContain(t, body, `hx-post="/agent/sessions/agent-x/apply/ui"`)
	if strings.Contains(body, "Apply unavailable") {
		t.Fatalf("did not expect disabled apply button when applicable, got: %s", body)
	}
	if strings.Contains(body, "Volver al inicio") {
		t.Fatalf("did not expect home link, got: %s", body)
	}
}

func TestPreviewMergeBar_NotApplicableHidesApplyButton(t *testing.T) {
	state := PreviewMergeBarState{
		Active:     true,
		SessionID:  "agent-x",
		BackURL:    "/agent?session=agent-x",
		Applicable: false,
	}
	body := renderPreviewMergeBar(t, state)
	mustContain(t, body, "Back to agent")
	if strings.Contains(body, "Apply preview") {
		t.Fatalf("did not expect apply button when not applicable, got: %s", body)
	}
	if strings.Contains(body, `hx-post`) {
		t.Fatalf("did not expect hx-post when not applicable, got: %s", body)
	}
}

func TestPreviewMergeBar_SuccessHidesApplyButtonKeepsBack(t *testing.T) {
	state := PreviewMergeBarState{
		Active:        true,
		SessionID:     "agent-x",
		BackURL:       "/agent?session=agent-x",
		BaseBranch:    "main",
		PreviewBranch: "agent/agent-x",
		Applicable:    true,
		Success:       true,
	}
	body := renderPreviewMergeBar(t, state)
	mustContain(t, body, "Applied")
	mustContain(t, body, "badge-success")
	mustContain(t, body, "Cambios aplicados a main.")
	mustContain(t, body, "Back to agent")
	if strings.Contains(body, "Apply preview") {
		t.Fatalf("did not expect apply button in success state, got: %s", body)
	}
	if strings.Contains(body, `hx-post`) {
		t.Fatalf("did not expect hx-post in success state, got: %s", body)
	}
}

func TestPreviewMergeBar_ErrorHidesApplyButtonKeepsBack(t *testing.T) {
	state := PreviewMergeBarState{
		Active:       true,
		SessionID:    "agent-x",
		BackURL:      "/agent?session=agent-x",
		BaseBranch:   "main",
		Applicable:   true,
		Success:      true,
		ErrorMessage: "No se pudo aplicar esta preview.",
	}
	body := renderPreviewMergeBar(t, state)
	mustContain(t, body, "badge-error")
	mustContain(t, body, "No se pudo aplicar esta preview.")
	mustContain(t, body, "Back to agent")
	if strings.Contains(body, "Apply preview") {
		t.Fatalf("did not expect apply button on error, got: %s", body)
	}
	if strings.Contains(body, "badge-success") {
		t.Fatalf("did not expect success badge when error present, got: %s", body)
	}
}

func TestPreviewMergeBar_StripAnchoredAtViewportBottom(t *testing.T) {
	state := PreviewMergeBarState{
		Active:    true,
		SessionID: "agent-x",
		BackURL:   "/agent?session=agent-x",
	}
	body := renderPreviewMergeBar(t, state)
	mustContain(t, body, "fixed inset-x-0 bottom-0")
}

func TestPreviewMergeBar_InactiveEmptiesContainer(t *testing.T) {
	state := PreviewMergeBarState{Active: false}
	body := renderPreviewMergeBar(t, state)
	mustContain(t, body, `<div id="global-preview-merge-bar"></div>`)
	if strings.Contains(body, "Back to agent") || strings.Contains(body, "Apply preview") {
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
