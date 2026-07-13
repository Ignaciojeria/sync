package layout

import "testing"

func TestPreviewMergeBarTone(t *testing.T) {
	cases := []struct {
		name string
		in   PreviewMergeBarState
		want string
	}{
		{"default", PreviewMergeBarState{}, "text-base-content"},
		{"success", PreviewMergeBarState{Success: true}, "text-success"},
		{"error overrides success", PreviewMergeBarState{Success: true, ErrorMessage: "boom"}, "text-error"},
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
		{"default", PreviewMergeBarState{}, "badge badge-info badge-sm"},
		{"success", PreviewMergeBarState{Success: true}, "badge badge-success badge-sm"},
		{"error", PreviewMergeBarState{ErrorMessage: "boom"}, "badge badge-error badge-sm"},
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
		{"success", PreviewMergeBarState{Success: true}, "Applied"},
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
		{"error overrides", PreviewMergeBarState{Success: true, ErrorMessage: "boom"}, "boom"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := previewMergeBarMessage(c.in)
			if !contains(got, c.want) {
				t.Errorf("message = %q, want substring %q", got, c.want)
			}
		})
	}
}

func TestPreviewApplyConfirm(t *testing.T) {
	withBranch := previewApplyConfirm(PreviewMergeBarState{BaseBranch: "main"})
	if !contains(withBranch, "main") {
		t.Errorf("with branch = %q", withBranch)
	}
	noBranch := previewApplyConfirm(PreviewMergeBarState{})
	if contains(noBranch, "branch") {
		t.Errorf("no branch should not mention branch: %q", noBranch)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
