package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentapp "scaffoldxd1/pkg/agent/application"
)

func TestPrepareCreatesSessionWorktree(t *testing.T) {
	repoRoot := initTestRepo(t)
	workspacesDir := filepath.Join(t.TempDir(), "workspaces")
	mgr := NewManager(repoRoot, workspacesDir)

	session, err := mgr.Prepare(context.Background(), agentapp.Session{
		ID:    "agent-123",
		Title: "t",
		CWD:   repoRoot,
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if got, want := session.Branch, "agent/agent-123"; got != want {
		t.Fatalf("Branch = %q, want %q", got, want)
	}
	if !strings.HasPrefix(session.WorkspacePath, workspacesDir) {
		t.Fatalf("WorkspacePath = %q, want prefix %q", session.WorkspacePath, workspacesDir)
	}
	if session.CWD != session.WorkspacePath {
		t.Fatalf("CWD = %q, want %q", session.CWD, session.WorkspacePath)
	}
	if _, err := os.Stat(filepath.Join(session.WorkspacePath, ".git")); err != nil {
		t.Fatalf("workspace .git: %v", err)
	}
	if _, err := os.Stat(filepath.Join(session.WorkspacePath, "README.md")); err != nil {
		t.Fatalf("workspace repo file: %v", err)
	}
}

func TestPrepareFallsBackToPlainCopyWhenNoGitRepo(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "hello.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	mgr := NewManager("", filepath.Join(t.TempDir(), "workspaces"))
	session, err := mgr.Prepare(context.Background(), agentapp.Session{ID: "agent-plain", CWD: source})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if got := session.Branch; got != "" {
		t.Fatalf("Branch = %q, want empty in plain-copy mode", got)
	}
	if _, err := os.Stat(filepath.Join(session.WorkspacePath, "hello.txt")); err != nil {
		t.Fatalf("plain workspace file: %v", err)
	}
}

func TestDestroyRemovesWorkspaceAndBranch(t *testing.T) {
	repoRoot := initTestRepo(t)
	mgr := NewManager(repoRoot, filepath.Join(t.TempDir(), "workspaces"))
	session, err := mgr.Prepare(context.Background(), agentapp.Session{ID: "agent-destroy", CWD: repoRoot})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if err := mgr.Destroy(context.Background(), session); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if _, err := os.Stat(session.WorkspacePath); !os.IsNotExist(err) {
		t.Fatalf("workspace todavía existe: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(session.WorkspacePath)); !os.IsNotExist(err) {
		t.Fatalf("contenedor de workspace todavía existe: %v", err)
	}
	if branchExists(context.Background(), repoRoot, session.Branch) {
		t.Fatalf("branch %q sigue existiendo", session.Branch)
	}
}

func TestPrepareKeepsRelativeSubdirInsideWorkspace(t *testing.T) {
	repoRoot := initTestRepo(t)
	subdir := filepath.Join(repoRoot, "pkg", "agent")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "hello.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write subdir file: %v", err)
	}
	mustGit(t, repoRoot, "add", ".")
	mustGit(t, repoRoot, "commit", "-m", "add subdir")

	mgr := NewManager(repoRoot, filepath.Join(t.TempDir(), "workspaces"))
	session, err := mgr.Prepare(context.Background(), agentapp.Session{ID: "agent-456", CWD: subdir})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if !strings.HasSuffix(session.CWD, filepath.Join("repo", "pkg", "agent")) {
		t.Fatalf("CWD = %q, esperaba terminar en repo/pkg/agent", session.CWD)
	}
	if _, err := os.Stat(filepath.Join(session.CWD, "hello.txt")); err != nil {
		t.Fatalf("workspace subdir file: %v", err)
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	mustGit(t, repoRoot, "init")
	mustGit(t, repoRoot, "config", "user.email", "test@example.com")
	mustGit(t, repoRoot, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	mustGit(t, repoRoot, "add", ".")
	mustGit(t, repoRoot, "commit", "-m", "init")
	return repoRoot
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if err := gitRun(context.Background(), dir, args...); err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
}
