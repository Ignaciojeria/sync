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
	if err := LoginPage("").Render(context.Background(), &buf); err != nil {
		t.Fatalf("LoginPage().Render() error = %v", err)
	}
	body := buf.String()
	checks := []string{
		"Continue with Google",
		"/auth/login/google",
		"Persistent cloud workspaces for AI agents",
		"Secure authentication powered by",
		"sync",
		">run</span>",
		"text-primary align-baseline text-[1.16em]",
		"viewBox=\"0 0 1600 900\"",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Fatalf("expected rendered login page to include %q, got %q", want, body)
		}
	}
}

func TestLoginPageRendersWithChildrenInContext(t *testing.T) {
	var buf bytes.Buffer
	// Set a non-nil children component in the context so the templ guard
	// `if templ_…_Var1 == nil` is exercised.
	ctx := templ.WithChildren(context.Background(), templ.Raw("<child/>"))
	if err := LoginPage("").Render(ctx, &buf); err != nil {
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
	if err := LoginPage("").Render(ctx, &buf); err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

// Verify that LoginPage is a templ.Component. Compile-time check.
var _ templ.Component = LoginPage("")

func TestAuthAppPath(t *testing.T) {
	cases := []struct {
		prefix, path, want string
	}{
		{"", "/foo", "/foo"},
		{"", "foo", "/foo"},
		{"/agent", "/foo", "/agent/foo"},
		{"/agent", "/", "/agent/"},
		{"  ", "/x", "/x"},
	}
	for _, c := range cases {
		if got := appPath(c.prefix, c.path); got != c.want {
			t.Errorf("appPath(%q, %q) = %q, want %q", c.prefix, c.path, got, c.want)
		}
	}
}
