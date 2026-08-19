package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureLocalProjectGitRemoved_OnlyRemovesDotGitInsideDir(t *testing.T) {
	tmp := t.TempDir()
	gitDir := filepath.Join(tmp, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(tmp, "keepme.txt")
	if err := os.WriteFile(sibling, []byte("don't touch"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ensureLocalProjectGitRemoved(tmp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(gitDir); !os.IsNotExist(err) {
		t.Fatalf("expected .git to be removed, got stat err: %v", err)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("sibling file should survive, got: %v", err)
	}
}

func TestEnsureLocalProjectGitRemoved_RejectsParentTraversal(t *testing.T) {
	// filepath.Join ya colapsa "..", así que probamos con un string crudo.
	if err := ensureLocalProjectGitRemoved(`C:\foo\..\etc`); err == nil {
		t.Fatal("expected error for path with '..', got nil")
	}
}

func TestEnsureLocalProjectGitRemoved_NoopWhenAbsent(t *testing.T) {
	tmp := t.TempDir()
	if err := ensureLocalProjectGitRemoved(tmp); err != nil {
		t.Fatalf("expected nil error when .git missing, got: %v", err)
	}
}
