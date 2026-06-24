package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestLoginPageRendersHtml(t *testing.T) {
	var buf bytes.Buffer
	if err := LoginPage().Render(context.Background(), &buf); err != nil {
		t.Fatalf("LoginPage().Render() error = %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, "Continue with Google") {
		t.Fatalf("expected rendered login page to include 'Continue with Google', got %q", body)
	}
	if !strings.Contains(body, "/auth/login/google") {
		t.Fatalf("expected rendered login page to link to /auth/login/google, got %q", body)
	}
}

func TestLoginPageRendersWithChildrenInContext(t *testing.T) {
	var buf bytes.Buffer
	// Set a non-nil children component in the context so the templ guard
	// `if templ_…_Var1 == nil` is exercised.
	ctx := templ.WithChildren(context.Background(), templ.Raw("<child/>"))
	if err := LoginPage().Render(ctx, &buf); err != nil {
		t.Fatalf("LoginPage().Render() error = %v", err)
	}
	if !strings.Contains(buf.String(), "Continue with Google") {
		t.Fatalf("expected rendered content, got %q", buf.String())
	}
}

func TestLoginPageRenderWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	if err := LoginPage().Render(ctx, &buf); err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

// Verify that LoginPage is a templ.Component. Compile-time check.
var _ templ.Component = LoginPage()
