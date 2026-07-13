package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentapp "fixtests1/internal/agent/application"
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
	if got, want := session.BaseBranch, repoHeadBranch(t, repoRoot); got != want {
		t.Fatalf("BaseBranch = %q, want %q", got, want)
	}
	if got, want := session.BaseCommit, repoHEAD(t, repoRoot); got != want {
		t.Fatalf("BaseCommit = %q, want %q", got, want)
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
	if got := session.BaseBranch; got != "" {
		t.Fatalf("BaseBranch = %q, want empty in plain-copy mode", got)
	}
	if got := session.BaseCommit; got != "" {
		t.Fatalf("BaseCommit = %q, want empty in plain-copy mode", got)
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

func TestApplyPreview_CopiesWorkspaceBackToSource(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "hello.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	mgr := NewManager("", filepath.Join(t.TempDir(), "workspaces"))
	session, err := mgr.Prepare(context.Background(), agentapp.Session{ID: "agent-apply", CWD: source})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if err := os.WriteFile(filepath.Join(session.WorkspacePath, "hello.txt"), []byte("preview\n"), 0o644); err != nil {
		t.Fatalf("write preview: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(session.WorkspacePath, "tmp", "run"), 0o755); err != nil {
		t.Fatalf("mkdir tmp: %v", err)
	}
	if err := os.WriteFile(filepath.Join(session.WorkspacePath, "tmp", "run", "skip.txt"), []byte("skip"), 0o644); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}
	result, err := mgr.ApplyPreview(context.Background(), session)
	if err != nil {
		t.Fatalf("ApplyPreview: %v", err)
	}
	if got, want := result.SourcePath, session.SourcePath; got != want {
		t.Fatalf("SourcePath = %q, want %q", got, want)
	}
	data, err := os.ReadFile(filepath.Join(source, "hello.txt"))
	if err != nil {
		t.Fatalf("read source after apply: %v", err)
	}
	if got := strings.ReplaceAll(string(data), "\r\n", "\n"); got != "preview\n" {
		t.Fatalf("hello.txt after apply = %q", got)
	}
	if _, err := os.Stat(filepath.Join(source, "tmp", "run", "skip.txt")); !os.IsNotExist(err) {
		t.Fatalf("tmp file copiado al source: %v", err)
	}
}

func TestMergePreview_MergesBranchIntoBase(t *testing.T) {
	repoRoot := initTestRepo(t)
	workspacesDir := filepath.Join(t.TempDir(), "workspaces")
	mgr := NewManager(repoRoot, workspacesDir)

	session, err := mgr.Prepare(context.Background(), agentapp.Session{ID: "agent-merge-ok", CWD: repoRoot})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if err := os.WriteFile(filepath.Join(session.WorkspacePath, "README.md"), []byte("hello merge\n"), 0o644); err != nil {
		t.Fatalf("write preview change: %v", err)
	}
	mustGit(t, session.WorkspacePath, "add", "README.md")
	mustGit(t, session.WorkspacePath, "commit", "-m", "preview change")

	result, err := mgr.MergePreview(context.Background(), session)
	if err != nil {
		t.Fatalf("MergePreview: %v", err)
	}
	if got, want := result.BaseBranch, session.BaseBranch; got != want {
		t.Fatalf("BaseBranch = %q, want %q", got, want)
	}
	if got, want := result.PreviewBranch, session.Branch; got != want {
		t.Fatalf("PreviewBranch = %q, want %q", got, want)
	}
	if strings.TrimSpace(result.Commit) == "" {
		t.Fatal("Commit vacío")
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, "README.md"))
	if err != nil {
		t.Fatalf("read merged file: %v", err)
	}
	if got := strings.ReplaceAll(string(data), "\r\n", "\n"); got != "hello merge\n" {
		t.Fatalf("README after merge = %q", got)
	}
}

func TestMergePreview_RejectsDirtyBaseRepo(t *testing.T) {
	repoRoot := initTestRepo(t)
	workspacesDir := filepath.Join(t.TempDir(), "workspaces")
	mgr := NewManager(repoRoot, workspacesDir)

	session, err := mgr.Prepare(context.Background(), agentapp.Session{ID: "agent-merge-dirty", CWD: repoRoot})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("dirty write: %v", err)
	}
	if _, err := mgr.MergePreview(context.Background(), session); err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("err = %v, want uncommitted changes", err)
	}
}

func TestMergePreview_AbortsOnConflict(t *testing.T) {
	repoRoot := initTestRepo(t)
	workspacesDir := filepath.Join(t.TempDir(), "workspaces")
	mgr := NewManager(repoRoot, workspacesDir)

	session, err := mgr.Prepare(context.Background(), agentapp.Session{ID: "agent-merge-conflict", CWD: repoRoot})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if err := os.WriteFile(filepath.Join(session.WorkspacePath, "README.md"), []byte("preview side\n"), 0o644); err != nil {
		t.Fatalf("preview write: %v", err)
	}
	mustGit(t, session.WorkspacePath, "add", "README.md")
	mustGit(t, session.WorkspacePath, "commit", "-m", "preview change")
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("base side\n"), 0o644); err != nil {
		t.Fatalf("base write: %v", err)
	}
	mustGit(t, repoRoot, "add", "README.md")
	mustGit(t, repoRoot, "commit", "-m", "base change")

	if _, err := mgr.MergePreview(context.Background(), session); err == nil || !strings.Contains(err.Error(), "merge") {
		t.Fatalf("err = %v, want merge conflict", err)
	}
	if dirty, err := repoDirty(context.Background(), repoRoot); err != nil {
		t.Fatalf("repoDirty: %v", err)
	} else if dirty {
		t.Fatal("repo quedó sucio tras abortar conflicto")
	}
}

func TestMarkMerged_SetsMetadata(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	session := agentapp.Session{}
	updated := MarkMerged(session, agentapp.MergeResult{Commit: "abc123", BaseBranch: "main", PreviewBranch: "agent/x"}, now)
	if updated.MergedAt == nil || !updated.MergedAt.Equal(now) {
		t.Fatalf("MergedAt = %v, want %v", updated.MergedAt, now)
	}
	if got, want := updated.MergedCommit, "abc123"; got != want {
		t.Fatalf("MergedCommit = %q, want %q", got, want)
	}
}

func TestPrepareKeepsRelativeSubdirInsideWorkspace(t *testing.T) {
	repoRoot := initTestRepo(t)
	subdir := filepath.Join(repoRoot, "internal", "agent")
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
	if !strings.HasSuffix(session.CWD, filepath.Join("repo", "internal", "agent")) {
		t.Fatalf("CWD = %q, esperaba terminar en repo/internal/agent", session.CWD)
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

func repoHeadBranch(t *testing.T, dir string) string {
	t.Helper()
	out, err := gitOutput(context.Background(), dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatalf("repo branch: %v", err)
	}
	return strings.TrimSpace(out)
}

func repoHEAD(t *testing.T, dir string) string {
	t.Helper()
	out, err := gitOutput(context.Background(), dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("repo head: %v", err)
	}
	return strings.TrimSpace(out)
}
