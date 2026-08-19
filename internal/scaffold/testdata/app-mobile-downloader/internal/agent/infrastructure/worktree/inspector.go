package worktree

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	agentapp "lastmile-agents/internal/agent/application"
)

// WorktreeSnapshot es la metadata que el panel muestra: branch,
// commits ahead/behind, files cambiados con diff stats, lista de
// commits, y total de lineas +/-. Se serializa a JSON y se devuelve
// al cliente para que el panel lo renderice.
//
// ponytail: NO incluir el diff completo aca. Un diff grande puede
// ser de 10k+ lineas. Eso se pide por separado via el endpoint
// /diff?file=PATH para que el panel pida el diff solo cuando el
// user hace click en un file.
type WorktreeSnapshot struct {
	Branch        string        `json:"branch"`
	BaseBranch    string        `json:"base_branch"`
	BaseCommit    string        `json:"base_commit"`
	CommitsAhead  int           `json:"commits_ahead"`
	CommitsBehind int           `json:"commits_behind"`
	FilesChanged  int           `json:"files_changed"`
	LinesAdded    int           `json:"lines_added"`
	LinesRemoved  int           `json:"lines_removed"`
	HasConflicts  bool          `json:"has_conflicts"`
	Files         []FileChange  `json:"files"`
	Commits       []CommitEntry `json:"commits"`
}

// FileChange describe un archivo modificado en el worktree.
// Status es el codigo de git status (M=modified, A=added, D=deleted,
// R=renamed, C=copied, ?=untracked). Additions/Deletions son las
// lineas +/- del diff.
type FileChange struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	OldPath   string `json:"old_path,omitempty"`
}

// CommitEntry describe un commit en el log del worktree.
type CommitEntry struct {
	Hash      string    `json:"hash"`
	ShortHash string    `json:"short_hash"`
	Message   string    `json:"message"`
	Author    string    `json:"author"`
	Timestamp time.Time `json:"timestamp"`
}

// FileDiff es el diff completo de un archivo. Se pide por separado
// cuando el user hace click en un file en el panel.
type FileDiff struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Content string `json:"content"`
}

// Inspector lee metadata del worktree de una sesion.
// Es un servicio read-only: no modifica nada en git. Solo lee
// status, log y diff para alimentar el panel del chat V2.
//
// ponytail: vive en este package (infrastructure/worktree) y no en
// application porque usa los git helpers (gitOutput, gitRun) que ya
// estan aca. Si lo pusieramos en application, tendriamos un ciclo
// de imports (application no puede importar infrastructure/worktree
// porque este ultimo ya importa application para usar el tipo
// agentapp.Session).
type Inspector struct{}

// NewInspector crea un inspector vacio. Stateless: todos los metodos
// reciben la sesion como parametro.
func NewInspector() *Inspector { return &Inspector{} }

// Inspect devuelve el snapshot del worktree de la sesion.
// Devuelve un error si la sesion no tiene worktree (Branch vacio)
// o si el directorio del worktree no existe.
func (i *Inspector) Inspect(ctx context.Context, session agentapp.Session) (WorktreeSnapshot, error) {
	if strings.TrimSpace(session.WorkspacePath) == "" {
		return WorktreeSnapshot{}, fmt.Errorf("worktree: session has no workspace path")
	}
	if strings.TrimSpace(session.Branch) == "" {
		return WorktreeSnapshot{}, fmt.Errorf("worktree: session is not a git worktree")
	}
	worktreePath := session.WorkspacePath

	// Paso 1: branch info.
	branch, err := gitOutput(ctx, worktreePath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return WorktreeSnapshot{}, fmt.Errorf("worktree: rev-parse: %w", err)
	}
	branch = strings.TrimSpace(branch)

	baseBranch := strings.TrimSpace(session.BaseBranch)
	if baseBranch == "" {
		baseBranch = "main"
	}
	baseCommit := strings.TrimSpace(session.BaseCommit)

	// Paso 2: commits ahead/behind.
	ahead, behind, err := gitAheadBehind(ctx, worktreePath, baseBranch)
	if err != nil {
		// ponytail: si baseBranch no existe localmente (caso raro),
		// caemos a 0/0 sin error para que el panel muestre el snapshot
		// igual. El usuario puede arreglar el baseBranch despues.
		ahead, behind = 0, 0
	}

	// Paso 3: files changed + diff stats.
	files, added, removed, conflicts, err := gitDiffStats(ctx, worktreePath, baseBranch)
	if err != nil {
		return WorktreeSnapshot{}, fmt.Errorf("worktree: diff stats: %w", err)
	}

	// Paso 4: commits.
	commits, err := gitCommits(ctx, worktreePath, baseBranch)
	if err != nil {
		return WorktreeSnapshot{}, fmt.Errorf("worktree: log: %w", err)
	}

	return WorktreeSnapshot{
		Branch:        branch,
		BaseBranch:    baseBranch,
		BaseCommit:    baseCommit,
		CommitsAhead:  ahead,
		CommitsBehind: behind,
		FilesChanged:  len(files),
		LinesAdded:    added,
		LinesRemoved:  removed,
		HasConflicts:  conflicts,
		Files:         files,
		Commits:       commits,
	}, nil
}

// Diff devuelve el diff completo de un archivo del worktree.
// Si el archivo fue agregado, devuelve el contenido completo.
// Si fue borrado, devuelve un header sin contenido.
// Si fue modificado, devuelve el diff unificado.
func (i *Inspector) Diff(ctx context.Context, session agentapp.Session, filePath string) (FileDiff, error) {
	if strings.TrimSpace(session.WorkspacePath) == "" || strings.TrimSpace(session.Branch) == "" {
		return FileDiff{}, fmt.Errorf("worktree: session is not a git worktree")
	}
	if strings.TrimSpace(filePath) == "" {
		return FileDiff{}, fmt.Errorf("worktree: file path is required")
	}
	baseBranch := strings.TrimSpace(session.BaseBranch)
	if baseBranch == "" {
		baseBranch = "main"
	}

	status, err := gitFileStatus(ctx, session.WorkspacePath, filePath, baseBranch)
	if err != nil {
		return FileDiff{}, fmt.Errorf("worktree: file status: %w", err)
	}

	var content string
	switch status {
	case "A":
		full, err := gitOutput(ctx, session.WorkspacePath, "show", "HEAD:"+filePath)
		if err != nil {
			return FileDiff{}, fmt.Errorf("worktree: show added: %w", err)
		}
		content = fmt.Sprintf("--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n%s",
			filePath, countLines(full), full)
	case "?":
		full, err := readUntrackedFile(session.WorkspacePath, filePath)
		if err != nil {
			return FileDiff{}, fmt.Errorf("worktree: read untracked: %w", err)
		}
		content = fmt.Sprintf("--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n%s",
			filePath, countLines(full), full)
	case "D":
		content = fmt.Sprintf("--- a/%s\n+++ /dev/null\n(deleted)\n", filePath)
	default:
		// M o R: diff unificado contra baseBranch.
		out, err := gitOutput(ctx, session.WorkspacePath, "diff", baseBranch+"..."+"HEAD", "--", filePath)
		if err != nil {
			return FileDiff{}, fmt.Errorf("worktree: diff: %w", err)
		}
		content = out
	}

	return FileDiff{
		Path:    filePath,
		Status:  status,
		Content: content,
	}, nil
}

// --- git helpers (internos) ----------------------------------------

// gitAheadBehind devuelve la cantidad de commits que el HEAD del
// worktree tiene adelante/atras de la base branch.
// Usa `git rev-list --count --left-right base...HEAD`:
// - LEFT (index 0) = commits en base que no estan en HEAD (behind).
// - RIGHT (index 1) = commits en HEAD que no estan en base (ahead).
func gitAheadBehind(ctx context.Context, dir, baseBranch string) (int, int, error) {
	out, err := gitOutput(ctx, dir, "rev-list", "--count", "--left-right", baseBranch+"...HEAD")
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("worktree: rev-list output: %q", out)
	}
	behind, _ := strconv.Atoi(fields[0])
	ahead, _ := strconv.Atoi(fields[1])
	return ahead, behind, nil
}

// gitDiffStats lee los archivos cambiados y sus lineas +/-.
// Combina `git diff --numstat` (para +/-) con `git diff --name-status`
// (para status M/A/D/R). Tambien incluye archivos untracked via
// `git ls-files --others --exclude-standard` para que el panel muestre
// archivos nuevos que el agent aun no commiteo.
//
// Devuelve conflicts=true si hay archivos con marcas de conflicto
// (<<<<<<<) sin resolver.
func gitDiffStats(ctx context.Context, dir, baseBranch string) ([]FileChange, int, int, bool, error) {
	numstatOut, err := gitOutput(ctx, dir, "diff", "--numstat", baseBranch+"...HEAD")
	if err != nil {
		return nil, 0, 0, false, err
	}
	stats := make(map[string][2]int)
	scanner := bufio.NewScanner(strings.NewReader(numstatOut))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			continue
		}
		added, _ := strconv.Atoi(fields[0])
		deleted, _ := strconv.Atoi(fields[1])
		// ponytail: para archivos binarios, git devuelve - - -.
		// Los contamos como 0 +/-. Se muestran igual en el panel.
		if added < 0 {
			added = 0
		}
		if deleted < 0 {
			deleted = 0
		}
		stats[fields[2]] = [2]int{added, deleted}
	}

	statusOut, err := gitOutput(ctx, dir, "diff", "--name-status", baseBranch+"...HEAD")
	if err != nil {
		return nil, 0, 0, false, err
	}
	var files []FileChange
	totalAdded, totalRemoved := 0, 0
	scanner = bufio.NewScanner(strings.NewReader(statusOut))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		status := fields[0]
		var path, oldPath string
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			// Rename/copy: old\tnew
			if len(fields) < 3 {
				continue
			}
			oldPath = fields[1]
			path = fields[2]
		} else {
			path = fields[1]
		}
		s := stats[path]
		files = append(files, FileChange{
			Path:      path,
			Status:    status,
			Additions: s[0],
			Deletions: s[1],
			OldPath:   oldPath,
		})
		totalAdded += s[0]
		totalRemoved += s[1]
	}

	// ponytail: archivos untracked. El agent puede haber creado
	// archivos nuevos sin hacer commit todavia. Los listamos para
	// que el panel muestre el estado completo del worktree.
	untrackedOut, _ := gitOutput(ctx, dir, "ls-files", "--others", "--exclude-standard")
	for _, line := range strings.Split(strings.TrimSpace(untrackedOut), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, FileChange{
			Path:      line,
			Status:    "?",
			Additions: 0,
			Deletions: 0,
		})
	}

	// Detect merge conflicts via `git diff --check`.
	conflicts := false
	checkOut, _ := gitOutput(ctx, dir, "diff", "--check", baseBranch+"...HEAD")
	if strings.Contains(checkOut, "conflict") {
		conflicts = true
	}

	return files, totalAdded, totalRemoved, conflicts, nil
}

// gitCommits lee la lista de commits entre baseBranch y HEAD.
// Cada commit tiene hash, short hash, message, author, timestamp.
func gitCommits(ctx context.Context, dir, baseBranch string) ([]CommitEntry, error) {
	// Format: hash\x00short\x00subject\x00author\x00timestamp-iso
	format := "--format=%H%x00%h%x00%s%x00%an%x00%aI"
	out, err := gitOutput(ctx, dir, "log", format, baseBranch+"..HEAD")
	if err != nil {
		return nil, err
	}
	var commits []CommitEntry
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x00")
		if len(fields) < 5 {
			continue
		}
		ts, _ := time.Parse(time.RFC3339, fields[4])
		commits = append(commits, CommitEntry{
			Hash:      fields[0],
			ShortHash: fields[1],
			Message:   fields[2],
			Author:    fields[3],
			Timestamp: ts,
		})
	}
	return commits, nil
}

// gitFileStatus devuelve el status (M/A/D/R/?) de un archivo
// especifico. Devuelve "?" si el archivo no esta en el diff pero
// esta untracked.
func gitFileStatus(ctx context.Context, dir, filePath, baseBranch string) (string, error) {
	out, err := gitOutput(ctx, dir, "diff", "--name-status", baseBranch+"...HEAD", "--", filePath)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(out) == "" {
		// No esta en el diff contra baseBranch — podria ser untracked.
		lsOut, _ := gitOutput(ctx, dir, "ls-files", "--others", "--exclude-standard", "--", filePath)
		if strings.TrimSpace(lsOut) != "" {
			return "?", nil
		}
		return "", fmt.Errorf("worktree: file %q not changed in worktree", filePath)
	}
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) < 2 {
		return "", fmt.Errorf("worktree: unexpected diff output: %q", out)
	}
	status := fields[0]
	if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
		status = string(status[0])
	}
	return status, nil
}

// readUntrackedFile lee el contenido de un archivo untracked del
// worktree. Lo usamos para mostrar el diff de archivos nuevos que
// el agent aun no commiteo.
func readUntrackedFile(worktreePath, filePath string) (string, error) {
	full := worktreePath + "/" + filePath
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
