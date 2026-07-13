package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestEditorViewRendersIframe(t *testing.T) {
	var buf bytes.Buffer
	if err := EditorView("").Render(context.Background(), &buf); err != nil {
		t.Fatalf("EditorView().Render() error = %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, `src="/editor"`) {
		t.Fatalf("expected iframe src to /editor, got %q", body)
	}
	if !strings.Contains(body, "sandbox=") {
		t.Fatalf("expected iframe to declare sandbox, got %q", body)
	}
}

func TestEditorAppPath(t *testing.T) {
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
