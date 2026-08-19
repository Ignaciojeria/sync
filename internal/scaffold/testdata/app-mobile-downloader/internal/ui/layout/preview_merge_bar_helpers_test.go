package layout

import (
	"strings"
	"testing"
)

func TestPreviewMergeBarTone(t *testing.T) {
	cases := []struct {
		name string
		in   PreviewMergeBarState
		want string
	}{
		// ponytail: success+NoChanges es estado neutral, no success
		// verde. El verde sólo lo merecemos cuando realmente
		// integramos commits.
		{"default", PreviewMergeBarState{}, "text-base-content"},
		{"success applies", PreviewMergeBarState{Success: true}, "text-success"},
		{"success noChanges neutral", PreviewMergeBarState{Success: true, NoChanges: true}, "text-base-content"},
		{"error overrides success", PreviewMergeBarState{Success: true, ErrorMessage: "boom"}, "text-error"},
		{"error overrides noChanges", PreviewMergeBarState{Success: true, NoChanges: true, ErrorMessage: "boom"}, "text-error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := previewMergeBarTone(c.in); got != c.want {
				t.Errorf("tone = %q, want %q", got, c.want)
			}
		})
	}
}

func TestPreviewMergeBarBadgeClass(t *testing.T) {
	cases := []struct {
		name string
		in   PreviewMergeBarState
		want string
	}{
		{"default preview", PreviewMergeBarState{}, "badge badge-info badge-sm"},
		{"success applies", PreviewMergeBarState{Success: true}, "badge badge-success badge-sm"},
		{"success noChanges uses info not success", PreviewMergeBarState{Success: true, NoChanges: true}, "badge badge-info badge-sm"},
		{"error", PreviewMergeBarState{ErrorMessage: "boom"}, "badge badge-error badge-sm"},
		{"error overrides success", PreviewMergeBarState{Success: true, ErrorMessage: "boom"}, "badge badge-error badge-sm"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := previewMergeBarBadgeClass(c.in); got != c.want {
				t.Errorf("badgeClass = %q, want %q", got, c.want)
			}
		})
	}
}

func TestPreviewMergeBarBadge(t *testing.T) {
	cases := []struct {
		name string
		in   PreviewMergeBarState
		want string
	}{
		{"default", PreviewMergeBarState{}, "Preview"},
		{"success applies", PreviewMergeBarState{Success: true}, "Applied"},
		{"success noChanges", PreviewMergeBarState{Success: true, NoChanges: true}, "Up to date"},
		{"error", PreviewMergeBarState{ErrorMessage: "boom"}, "Blocked"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := previewMergeBarBadge(c.in); got != c.want {
				t.Errorf("badge = %q, want %q", got, c.want)
			}
		})
	}
}

func TestPreviewMergeBarMessage(t *testing.T) {
	cases := []struct {
		name string
		in   PreviewMergeBarState
		want string
	}{
		{"default preview message", PreviewMergeBarState{}, "Preview aislada"},
		{"success with branch", PreviewMergeBarState{Success: true, BaseBranch: "main"}, "Cambios aplicados a main."},
		{"success without branch", PreviewMergeBarState{Success: true}, "Cambios aplicados."},
		{"success noChanges", PreviewMergeBarState{Success: true, NoChanges: true}, "No hay cambios nuevos para mergear."},
		{"error overrides", PreviewMergeBarState{Success: true, ErrorMessage: "boom"}, "boom"},
		{"error overrides noChanges", PreviewMergeBarState{Success: true, NoChanges: true, ErrorMessage: "boom"}, "boom"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := previewMergeBarMessage(c.in)
			if !strings.Contains(got, c.want) {
				t.Errorf("message = %q, want substring %q", got, c.want)
			}
		})
	}
}

func TestPreviewMergeConfirm(t *testing.T) {
	withBranches := previewMergeConfirm(PreviewMergeBarState{PreviewBranch: "agent/x", BaseBranch: "main"})
	if !strings.Contains(withBranches, "agent/x") || !strings.Contains(withBranches, "main") {
		t.Errorf("with branches = %q, want both branch names", withBranches)
	}
	baseOnly := previewMergeConfirm(PreviewMergeBarState{PreviewBranch: "agent/x"})
	if !strings.Contains(baseOnly, "agent/x") {
		t.Errorf("with preview only = %q, want preview name", baseOnly)
	}
	plain := previewMergeConfirm(PreviewMergeBarState{})
	if strings.TrimSpace(plain) == "" {
		t.Errorf("plain confirm should not be empty")
	}
}
