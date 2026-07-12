package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	agentapp "testboi1/pkg/agent/application"
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
	baseBranch, err := currentBranch(ctx, repoRoot)
	if err != nil {
		return session, err
	}
	baseCommit, err := currentCommit(ctx, repoRoot)
	if err != nil {
		return session, err
	}

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

	return applyWorkspaceSession(session, workspaceRepoPath, repoRoot, relSubdir, branch, baseBranch, baseCommit)
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
	return applyWorkspaceSession(session, workspaceRepoPath, targetPath, "", "", "", "")
}

func applyWorkspaceSession(session agentapp.Session, workspaceRepoPath, sourcePath, relSubdir, branch, baseBranch, baseCommit string) (agentapp.Session, error) {
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
	session.SourcePath = strings.TrimSpace(sourcePath)
	session.CWD = workspaceCWD
	session.Branch = branch
	session.BaseBranch = strings.TrimSpace(baseBranch)
	session.BaseCommit = strings.TrimSpace(baseCommit)
	return session, nil
}

func currentBranch(ctx context.Context, repoRoot string) (string, error) {
	out, err := gitOutput(ctx, repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("worktree: current branch: %w", err)
	}
	return strings.TrimSpace(out), nil
}

func currentCommit(ctx context.Context, repoRoot string) (string, error) {
	out, err := gitOutput(ctx, repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("worktree: current commit: %w", err)
	}
	return strings.TrimSpace(out), nil
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
	return copyTreeFiltered(src, dst, func(name string) bool {
		return name == ".git"
	})
}

func copyTreeFiltered(src, dst string, skip func(name string) bool) error {
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
		if skip != nil && skip(entry.Name()) {
			continue
		}
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyTreeFiltered(srcPath, dstPath, skip); err != nil {
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
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
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

func (m *Manager) ApplyPreview(_ context.Context, session agentapp.Session) (agentapp.ApplyResult, error) {
	sourcePath := strings.TrimSpace(session.SourcePath)
	previewPath := strings.TrimSpace(session.WorkspacePath)
	if sourcePath == "" || previewPath == "" {
		return agentapp.ApplyResult{}, fmt.Errorf("worktree: session is not applicable")
	}
	if err := copyTreeFiltered(previewPath, sourcePath, shouldSkipApplyPath); err != nil {
		return agentapp.ApplyResult{}, fmt.Errorf("worktree: apply preview: %w", err)
	}
	return agentapp.ApplyResult{SourcePath: sourcePath, PreviewPath: previewPath}, nil
}

func shouldSkipApplyPath(name string) bool {
	switch strings.TrimSpace(name) {
	case ".git", "tmp", ".air.log", "api.exe", "preview-main", "preview-main.exe":
		return true
	default:
		return false
	}
}

func (m *Manager) MergePreview(ctx context.Context, session agentapp.Session) (agentapp.MergeResult, error) {
	repoRoot, err := m.repoRootForSession(ctx, session)
	if err != nil {
		return agentapp.MergeResult{}, err
	}
	previewBranch := strings.TrimSpace(session.Branch)
	baseBranch := strings.TrimSpace(session.BaseBranch)
	if previewBranch == "" || baseBranch == "" {
		return agentapp.MergeResult{}, fmt.Errorf("worktree: session is not mergeable")
	}
	if session.MergedAt != nil || strings.TrimSpace(session.MergedCommit) != "" {
		return agentapp.MergeResult{}, fmt.Errorf("worktree: preview already merged")
	}
	if dirty, err := repoDirty(ctx, repoRoot); err != nil {
		return agentapp.MergeResult{}, err
	} else if dirty {
		return agentapp.MergeResult{}, fmt.Errorf("worktree: base repo has uncommitted changes")
	}
	if err := gitRun(ctx, repoRoot, "checkout", baseBranch); err != nil {
		return agentapp.MergeResult{}, fmt.Errorf("worktree: checkout base branch %q: %w", baseBranch, err)
	}
	if err := gitRun(ctx, repoRoot, "merge", "--no-ff", "--no-edit", previewBranch); err != nil {
		_ = gitRun(ctx, repoRoot, "merge", "--abort")
		return agentapp.MergeResult{}, fmt.Errorf("worktree: merge %q into %q: %w", previewBranch, baseBranch, err)
	}
	commit, err := currentCommit(ctx, repoRoot)
	if err != nil {
		return agentapp.MergeResult{}, err
	}
	return agentapp.MergeResult{BaseBranch: baseBranch, PreviewBranch: previewBranch, Commit: commit}, nil
}

func repoDirty(ctx context.Context, repoRoot string) (bool, error) {
	out, err := gitOutput(ctx, repoRoot, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("worktree: repo status: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}

func (m *Manager) repoRootForSession(ctx context.Context, session agentapp.Session) (string, error) {
	workspacePath := strings.TrimSpace(session.WorkspacePath)
	if workspacePath != "" && strings.TrimSpace(session.Branch) != "" {
		if root, err := baseRepoRootFromWorktree(ctx, workspacePath, session.BaseBranch); err == nil {
			return root, nil
		}
	}
	if workspacePath != "" {
		if root, err := gitOutput(ctx, workspacePath, "rev-parse", "--show-toplevel"); err == nil {
			return strings.TrimSpace(root), nil
		}
	}
	cwd := strings.TrimSpace(session.CWD)
	if cwd != "" {
		return m.resolveRepoRoot(ctx, cwd)
	}
	return "", fmt.Errorf("worktree: session has no repo root")
}

func baseRepoRootFromWorktree(ctx context.Context, workspacePath, baseBranch string) (string, error) {
	commonDir, err := gitOutput(ctx, workspacePath, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	commonDir = strings.TrimSpace(commonDir)
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(workspacePath, commonDir)
	}
	out, err := gitOutputWithGitDir(ctx, commonDir, "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}

	var currentPath string
	var currentBranch string
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if currentPath != "" && (currentBranch == "refs/heads/"+strings.TrimSpace(baseBranch) || currentBranch == "") {
				return currentPath, nil
			}
			currentPath = ""
			currentBranch = ""
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
			continue
		}
		if strings.HasPrefix(line, "branch ") {
			currentBranch = strings.TrimSpace(strings.TrimPrefix(line, "branch "))
		}
	}
	if currentPath != "" && (currentBranch == "refs/heads/"+strings.TrimSpace(baseBranch) || currentBranch == "") {
		return currentPath, nil
	}
	return "", fmt.Errorf("worktree: base repo root not found")
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

func MarkMerged(session agentapp.Session, result agentapp.MergeResult, now time.Time) agentapp.Session {
	if now.IsZero() {
		now = time.Now()
	}
	session.MergedAt = &now
	session.MergedCommit = strings.TrimSpace(result.Commit)
	if strings.TrimSpace(result.BaseBranch) != "" {
		session.BaseBranch = strings.TrimSpace(result.BaseBranch)
	}
	if strings.TrimSpace(result.PreviewBranch) != "" {
		session.Branch = strings.TrimSpace(result.PreviewBranch)
	}
	return session
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

func gitOutputWithGitDir(ctx context.Context, gitDir string, args ...string) (string, error) {
	allArgs := append([]string{"--git-dir", gitDir}, args...)
	cmd := exec.CommandContext(ctx, "git", allArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(allArgs, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
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
