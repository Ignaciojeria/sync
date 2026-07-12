//go:build linux

package application

import (
	"context"
	"path/filepath"
	"testing"
)

func TestKillProcessesInWorkspace_EmptyPath(t *testing.T) {
	n, err := KillProcessesInWorkspace(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 kills for empty path, got %d", n)
	}
}

func TestKillProcessesInWorkspace_WhitespacePath(t *testing.T) {
	n, err := KillProcessesInWorkspace(context.Background(), "   \t\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 kills for whitespace path, got %d", n)
	}
}

func TestKillProcessesInWorkspace_NonexistentPath(t *testing.T) {
	n, err := KillProcessesInWorkspace(context.Background(), filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("expected nil error for nonexistent path, got: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 kills for nonexistent path, got %d", n)
	}
}

func TestKillProcessesInWorkspace_NoProcessesInside(t *testing.T) {
	// Workspace vacío: no debería matar nada, ni siquiera el proceso
	// de test (cuyo CWD está afuera).
	dir := t.TempDir()
	n, err := KillProcessesInWorkspace(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 kills for empty workspace, got %d", n)
	}
}