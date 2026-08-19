package agent

import (
	"errors"
	"testing"

	"lastmile-agents/internal/shared/server"
)

func TestPreviewOwnerSessionIDFromMountPrefix(t *testing.T) {
	if got, want := previewOwnerSessionIDFromMountPrefix("/agent/sessions/p1/preview/"), "p1"; got != want {
		t.Fatalf("owner id = %q, want %q", got, want)
	}
	if got := previewOwnerSessionIDFromMountPrefix("/agent"); got != "" {
		t.Fatalf("owner id = %q, want empty", got)
	}
}

// ponytail: cuando journalPersistUserPrompt falla (disco lleno,
// permisos, etc.), el POST debe responder 503 para que el cliente
// sepa que tiene que reintentar. Si lo silenciamos, el user_prompt
// "existe" en la respuesta del runtime pero NO queda en el
// journal de replay — al refrescar, la UI muestra la respuesta
// del assistant sin la pregunta. Ver internal/agent/http/prompt.go.
func TestMapSessionError_JournalPersist(t *testing.T) {
	base := errors.New("disk full")
	jerr := &journalPersistError{cause: base}
	mapped := mapSessionError(jerr)
	httpErr, ok := mapped.(server.HTTPError)
	if !ok {
		t.Fatalf("mapped = %T, want server.HTTPError", mapped)
	}
	if httpErr.Status != 503 {
		t.Fatalf("status = %d, want 503", httpErr.Status)
	}
	if !strings_contains(httpErr.Detail, "intentá nuevamente") {
		t.Fatalf("detail = %q, want hint to retry", httpErr.Detail)
	}
}

func strings_contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
