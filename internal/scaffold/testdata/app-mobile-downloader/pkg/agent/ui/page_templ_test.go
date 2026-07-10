package ui

import (
	"bytes"
	"testing"
)

func TestPageRenders(t *testing.T) {
	var buf bytes.Buffer
	if err := Page(PageState{}).Render(t.Context(), &buf); err != nil {
		t.Fatalf("render page: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected rendered markup")
	}
}

func TestProvidersPageRenders(t *testing.T) {
	var buf bytes.Buffer
	if err := ProvidersPage(PageState{CurrentView: "providers"}).Render(t.Context(), &buf); err != nil {
		t.Fatalf("render providers page: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected rendered providers markup")
	}
}
