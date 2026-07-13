package application

import (
	"encoding/json"
	"testing"
)

func TestExtractAssistantStopError_InsufficientCredits(t *testing.T) {
	raw := json.RawMessage(`{"message":{"role":"assistant","stopReason":"error","errorMessage":"402 \"insufficient_credits\""}}`)
	got, ok := extractAssistantStopError(raw)
	if !ok {
		t.Fatal("expected ok=true")
	}
	want := "Créditos insuficientes en el proveedor/modelo configurado."
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
