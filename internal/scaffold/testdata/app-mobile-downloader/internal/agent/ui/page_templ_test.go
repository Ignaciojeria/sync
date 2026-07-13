package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/a-h/templ"
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

func TestStandalonePageRenders(t *testing.T) {
	var buf bytes.Buffer
	if err := StandalonePage(PageState{}, "ocean", "/theme.css").Render(t.Context(), &buf); err != nil {
		t.Fatalf("render standalone: %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, "<!doctype html>") {
		t.Fatalf("missing doctype: %q", body)
	}
	if !strings.Contains(body, "data-theme=\"ocean\"") {
		t.Fatalf("missing theme id: %q", body)
	}
}

func TestStandalonePageWithContentRenders(t *testing.T) {
	var buf bytes.Buffer
	if err := StandalonePageWithContent(PageState{}, "sunset", "/t.css", templ.Raw("<child/>")).Render(t.Context(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, "data-theme=\"sunset\"") {
		t.Fatalf("missing theme: %q", body)
	}
	if !strings.Contains(body, "<child/>") {
		t.Fatalf("missing child content: %q", body)
	}
}
