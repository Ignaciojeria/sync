package agent

import "testing"

func TestPreviewOwnerSessionIDFromMountPrefix(t *testing.T) {
	if got, want := previewOwnerSessionIDFromMountPrefix("/agent/sessions/p1/preview/"), "p1"; got != want {
		t.Fatalf("owner id = %q, want %q", got, want)
	}
	if got := previewOwnerSessionIDFromMountPrefix("/agent"); got != "" {
		t.Fatalf("owner id = %q, want empty", got)
	}
}
