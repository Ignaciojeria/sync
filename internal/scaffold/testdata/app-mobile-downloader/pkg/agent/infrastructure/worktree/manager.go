package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	agentapp "scaffoldxd1/pkg/agent/application"
)

// Manager prepara un workspace Git aislado por sesión.
//
// ponytail: concreto, sin interfaces todavía. Cuando exista un segundo
// backend real, recién extraemos el contrato.
type Manager struct {
	RepoRoot      string
	WorkspacesDir string
}

func NewManager(repoRoot, workspacesDir string) *Manager {
	return &Manager{
		RepoRoot:      strings.TrimSpace(repoRoot),
		WorkspacesDir: strings.TrimSpace(workspacesDir),
	}
}

func (m *Manager) Prepare(ctx context.Context, session agentapp.Session) (agentapp.Session, error) {
	targetPath, err := resolveTargetPath(session.CWD)
	if err != nil {
		return session, err
	}
	repoRoot, err := m.resolveRepoRoot(ctx, targetPath)
	if err != nil {
		if isNotGitRepoErr(err) {
			return m.preparePlainWorkspace(targetPath, session)
		}
		return session, err
	}
	workspaceRoot, err := m.resolveWorkspacesDir(repoRoot)
	if err != nil {
		return session, err
	}
	workspaceRepoPath := filepath.Join(workspaceRoot, sanitizeSessionID(session.ID), "repo")
	branch := branchName(session.ID)

	relSubdir, err := filepath.Rel(repoRoot, targetPath)
	if err != nil {
		return session, fmt.Errorf("worktree: rel %q from %q: %w", targetPath, repoRoot, err)
	}
	if relSubdir == "." {
		relSubdir = ""
	}

	if err := m.ensureWorktree(ctx, repoRoot, workspaceRepoPath, branch); err != nil {
		return session, err
	}

	return applyWorkspaceSession(session, workspaceRepoPath, relSubdir, branch)
}

func resolveTargetPath(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || spec == "." || spec == "./" {
		spec = "."
	}
	abs, err := filepath.Abs(spec)
	if err != nil {
		return "", fmt.Errorf("worktree: abs target %q: %w", spec, err)
	}
	return abs, nil
}

func (m *Manager) resolveRepoRoot(ctx context.Context, targetPath string) (string, error) {
	if root := strings.TrimSpace(m.RepoRoot); root != "" {
		abs, err := filepath.Abs(root)
		if err != nil {
			return "", fmt.Errorf("worktree: abs repo root %q: %w", root, err)
		}
		return abs, nil
	}
	out, err := gitOutput(ctx, targetPath, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("worktree: repo root for %q: %w", targetPath, err)
	}
	return strings.TrimSpace(out), nil
}

func (m *Manager) resolveWorkspacesDir(repoRoot string) (string, error) {
	if dir := strings.TrimSpace(m.WorkspacesDir); dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", fmt.Errorf("worktree: abs workspaces dir %q: %w", dir, err)
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return "", fmt.Errorf("worktree: mkdir %q: %w", abs, err)
		}
		return abs, nil
	}
	parent := filepath.Dir(repoRoot)
	name := "." + filepath.Base(repoRoot) + "-agent-workspaces"
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("worktree: mkdir %q: %w", dir, err)
	}
	return dir, nil
}

func (m *Manager) ensureWorktree(ctx context.Context, repoRoot, workspaceRepoPath, branch string) error {
	if fi, err := os.Stat(filepath.Join(workspaceRepoPath, ".git")); err == nil && !fi.IsDir() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(workspaceRepoPath), 0o755); err != nil {
		return fmt.Errorf("worktree: mkdir parent %q: %w", filepath.Dir(workspaceRepoPath), err)
	}
	if branchExists(ctx, repoRoot, branch) {
		if err := gitRun(ctx, repoRoot, "worktree", "add", workspaceRepoPath, branch); err != nil {
			return fmt.Errorf("worktree: add existing branch %q: %w", branch, err)
		}
		return nil
	}
	if err := gitRun(ctx, repoRoot, "worktree", "add", "-b", branch, workspaceRepoPath, "HEAD"); err != nil {
		return fmt.Errorf("worktree: create branch %q: %w", branch, err)
	}
	return nil
}

func (m *Manager) preparePlainWorkspace(targetPath string, session agentapp.Session) (agentapp.Session, error) {
	workspaceRoot, err := m.resolveWorkspacesDir(targetPath)
	if err != nil {
		return session, err
	}
	workspaceRepoPath := filepath.Join(workspaceRoot, sanitizeSessionID(session.ID), "repo")
	if err := os.RemoveAll(workspaceRepoPath); err != nil {
		return session, fmt.Errorf("worktree: clean plain workspace %q: %w", workspaceRepoPath, err)
	}
	if err := copyDir(targetPath, workspaceRepoPath); err != nil {
		return session, fmt.Errorf("worktree: copy plain workspace from %q: %w", targetPath, err)
	}
	return applyWorkspaceSession(session, workspaceRepoPath, "", "")
}

func applyWorkspaceSession(session agentapp.Session, workspaceRepoPath, relSubdir, branch string) (agentapp.Session, error) {
	var err error
	workspaceCWD := workspaceRepoPath
	if relSubdir != "" {
		workspaceCWD = filepath.Join(workspaceRepoPath, relSubdir)
	}
	workspaceRepoPath, err = filepath.Abs(workspaceRepoPath)
	if err != nil {
		return session, fmt.Errorf("worktree: abs workspace: %w", err)
	}
	workspaceCWD, err = filepath.Abs(workspaceCWD)
	if err != nil {
		return session, fmt.Errorf("worktree: abs cwd: %w", err)
	}
	if _, err := os.Stat(workspaceCWD); err != nil {
		return session, fmt.Errorf("worktree: target subdir %q missing in workspace: %w", workspaceCWD, err)
	}
	session.WorkspacePath = workspaceRepoPath
	session.CWD = workspaceCWD
	session.Branch = branch
	return session, nil
}

func branchExists(ctx context.Context, repoRoot, branch string) bool {
	err := gitRun(ctx, repoRoot, "rev-parse", "--verify", "refs/heads/"+branch)
	return err == nil
}

func gitRun(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func isNotGitRepoErr(err error) bool {
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "not a git repository")
}

func copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := os.WriteFile(dstPath, data, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func sanitizeSessionID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "session"
	}
	var b strings.Builder
	b.Grow(len(id))
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-._")
	if out == "" {
		return "session"
	}
	return out
}

func (m *Manager) Destroy(ctx context.Context, session agentapp.Session) error {
	workspacePath := strings.TrimSpace(session.WorkspacePath)
	if workspacePath == "" {
		workspacePath = strings.TrimSpace(session.CWD)
	}
	branch := strings.TrimSpace(session.Branch)
	workspaceRoot := workspaceContainerDir(workspacePath)
	if branch != "" && workspacePath != "" {
		commonDir, err := gitOutput(ctx, workspacePath, "rev-parse", "--git-common-dir")
		if err == nil {
			commonDir = strings.TrimSpace(commonDir)
			if !filepath.IsAbs(commonDir) {
				commonDir = filepath.Join(workspacePath, commonDir)
			}
			_ = gitRun(ctx, workspacePath, "worktree", "remove", "--force", workspacePath)
			_ = gitRunWithGitDir(ctx, commonDir, "branch", "-D", branch)
		}
		_ = os.RemoveAll(workspacePath)
	} else if workspacePath != "" {
		_ = os.RemoveAll(workspacePath)
	}
	if workspaceRoot != "" {
		_ = os.RemoveAll(workspaceRoot)
	}
	if session.PiSessionFile != "" {
		_ = os.Remove(session.PiSessionFile)
	}
	return nil
}

func gitRunWithGitDir(ctx context.Context, gitDir string, args ...string) error {
	allArgs := append([]string{"--git-dir", gitDir}, args...)
	cmd := exec.CommandContext(ctx, "git", allArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(allArgs, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func workspaceContainerDir(workspacePath string) string {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return ""
	}
	if filepath.Base(workspacePath) == "repo" {
		return filepath.Dir(workspacePath)
	}
	return workspacePath
}

func branchName(sessionID string) string {
	return "agent/" + sanitizeSessionID(sessionID)
}
