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
		ActionPath:    "/agent/sessions/agent-x/merge/ui",
	}
	body := renderPreviewMergeBar(t, state)
	mustContain(t, body, "badge-info")
	mustContain(t, body, "Preview")
	mustContain(t, body, "Preview aislada")
	mustContain(t, body, `href="/agent?session=agent-x"`)
	mustContain(t, body, "Back to agent")
	mustContain(t, body, "Merge preview")
	mustContain(t, body, `hx-post="/agent/sessions/agent-x/merge/ui"`)
	if strings.Contains(body, "Apply preview") {
		t.Fatalf("did not expect apply label in merge-only bar, got: %s", body)
	}
}

func TestPreviewMergeBar_NotApplicableHidesMergeButton(t *testing.T) {
	state := PreviewMergeBarState{
		Active:     true,
		SessionID:  "agent-x",
		BackURL:    "/agent?session=agent-x",
		Applicable: false,
	}
	body := renderPreviewMergeBar(t, state)
	mustContain(t, body, "Back to agent")
	if strings.Contains(body, "Merge preview") {
		t.Fatalf("did not expect merge button when not applicable, got: %s", body)
	}
	if strings.Contains(body, `hx-post`) {
		t.Fatalf("did not expect hx-post when not applicable, got: %s", body)
	}
}

func TestPreviewMergeBar_SuccessAppliedHidesMergeShowsBadge(t *testing.T) {
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
	if strings.Contains(body, "Merge preview") {
		t.Fatalf("did not expect merge button in success state, got: %s", body)
	}
}

func TestPreviewMergeBar_SuccessNoChangesShowsUpToDate(t *testing.T) {
	// ponytail: cuando el merge integró cero commits nuevos, el bar
	// muestra "Up to date" (badge info, no success), mensaje neutro,
	// y sigue dejando el botón "Back to agent".
	state := PreviewMergeBarState{
		Active:        true,
		SessionID:     "agent-x",
		BackURL:       "/agent?session=agent-x",
		BaseBranch:    "main",
		PreviewBranch: "agent/agent-x",
		Applicable:    true,
		Success:       true,
		NoChanges:     true,
	}
	body := renderPreviewMergeBar(t, state)
	mustContain(t, body, "Up to date")
	mustContain(t, body, "No hay cambios nuevos para mergear.")
	mustContain(t, body, "Back to agent")
	if strings.Contains(body, "badge-success") {
		t.Fatalf("noChanges must NOT use success badge, got: %s", body)
	}
	if strings.Contains(body, "Merge preview") {
		t.Fatalf("did not expect merge button in success state, got: %s", body)
	}
}

func TestPreviewMergeBar_ErrorHidesMergeButtonKeepsBack(t *testing.T) {
	state := PreviewMergeBarState{
		Active:       true,
		SessionID:    "agent-x",
		BackURL:      "/agent?session=agent-x",
		BaseBranch:   "main",
		Applicable:   true,
		Success:      true,
		ErrorMessage: "Conflicto de merge entre la preview y el branch base.",
	}
	body := renderPreviewMergeBar(t, state)
	mustContain(t, body, "badge-error")
	mustContain(t, body, "Conflicto de merge")
	mustContain(t, body, "Back to agent")
	if strings.Contains(body, "Merge preview") {
		t.Fatalf("did not expect merge button on error, got: %s", body)
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
	if strings.Contains(body, "Back to agent") || strings.Contains(body, "Merge preview") {
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
