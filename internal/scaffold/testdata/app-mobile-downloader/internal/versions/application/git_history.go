// Package versionsapp implementa el servicio que lee el historial
// de merges del repositorio local y lo expone como "versiones"
// desplegadas. Cada merge de un worktree en la rama principal es
// una versión, porque air recompila el binario tras el merge.
package versionsapp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Reader es el contrato que consume la capa http. Vive acá para
// permitir mocks en tests y para invertir la dependencia (la ui/http
// depende de la interfaz, no de la implementación concreta).
type Reader interface {
	List(ctx context.Context, limit int) ([]Version, error)
	Get(ctx context.Context, sha string) (Version, error)
	Diff(ctx context.Context, sha string) ([]VersionFile, error)
}

// GitHistory implementa Reader ejecutando comandos git contra el
// repositorio configurado en RepoPath. Mantenemos la implementación
// sin dependencias externas (sin go-git) para minimizar la superficie
// del módulo y porque git ya está garantizado en el entorno.
type GitHistory struct {
	RepoPath string
	// Branch principal contra la que se mergean los worktrees. Por
	// defecto usamos "main" pero se permite override para setups con
	// "master", "develop", etc.
	MainBranch string
}

// New construye un GitHistory con defaults razonables.
func New(repoPath string) *GitHistory {
	if strings.TrimSpace(repoPath) == "" {
		repoPath = "."
	}
	return &GitHistory{RepoPath: repoPath, MainBranch: "main"}
}

// List devuelve los últimos `limit` commits de merge en la rama
// principal. Cada elemento es una "versión" según la convención del
// módulo: merge de worktree -> redeploy -> nueva versión corriendo.
func (g *GitHistory) List(ctx context.Context, limit int) ([]Version, error) {
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	// %H full sha, %h short, %s subject, %an author, %ae email,
	// %at author time (unix), %P parents. Usamos --merges para
	// quedarnos solo con commits de merge (cada merge de worktree
	// genera uno en main).
	format := "%H%x1f%h%x1f%s%x1f%an%x1f%ae%x1f%at%x1f%P"
	args := []string{
		"log", "--merges", "--first-parent",
		"-n", strconv.Itoa(limit),
		"--pretty=format:" + format,
		g.MainBranch,
	}
	out, err := g.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	head, err := g.headSHA(ctx)
	if err != nil {
		return nil, err
	}
	lines := splitLines(out)
	result := make([]Version, 0, len(lines))
	for _, line := range lines {
		v, ok := parseVersion(line)
		if !ok {
			continue
		}
		v.IsCurrent = v.SHA == head
		// %P devuelve los parents del merge como SHAs, no nombres.
		// Para mostrar el branch origen en la UI parseamos el mensaje:
		// git genera "Merge branch 'X' into Y" en merges típicos.
		v.Branch = extractBranchFromMessage(v.Message)
		result = append(result, v)
	}
	return result, nil
}

// Get devuelve el detalle de una versión a partir de su SHA (completo
// o corto). Devuelve error si el SHA no existe en el repo.
func (g *GitHistory) Get(ctx context.Context, sha string) (Version, error) {
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return Version{}, errors.New("versions: sha vacío")
	}
	format := "%H%x1f%h%x1f%s%x1f%an%x1f%ae%x1f%at%x1f%P"
	out, err := g.run(ctx, "log", "-n", "1", "--pretty=format:"+format, sha)
	if err != nil {
		return Version{}, fmt.Errorf("versions: sha %q no encontrado: %w", sha, err)
	}
	head, err := g.headSHA(ctx)
	if err != nil {
		return Version{}, err
	}
	line := strings.TrimSpace(out)
	v, ok := parseVersion(line)
	if !ok {
		return Version{}, fmt.Errorf("versions: no se pudo parsear sha %q", sha)
	}
	v.IsCurrent = v.SHA == head
	v.Branch = extractBranchFromMessage(v.Message)
	return v, nil
}

// Diff lista los archivos cambiados entre la versión dada y HEAD,
// con un preview corto del diff para mostrar en la vista detalle.
func (g *GitHistory) Diff(ctx context.Context, sha string) ([]VersionFile, error) {
	if strings.TrimSpace(sha) == "" {
		return nil, errors.New("versions: sha vacío")
	}
	// --stat da adds/dels por archivo; --numstat es más parseable.
	out, err := g.run(ctx, "diff", "--numstat", "--no-renames", sha+"~1", sha)
	if err != nil {
		// Fallback: algunos merges no tienen parent útil para el
		// diff tradicional (ej. octopus merges). En ese caso usamos
		// --stat sin contexto y mostramos solo el nombre del archivo.
		out, err = g.run(ctx, "show", "--stat", "--format=", sha)
		if err != nil {
			return nil, err
		}
	}
	files := parseNumstat(out)
	for i := range files {
		// Pedimos un diff unitario por archivo para el preview. Si
		// falla (binario, archivo borrado, etc.) dejamos Preview="".
		preview, _ := g.singleFileDiff(ctx, sha, files[i].Path)
		files[i].Preview = preview
	}
	return files, nil
}

func (g *GitHistory) singleFileDiff(ctx context.Context, sha, path string) (string, error) {
	out, err := g.run(ctx, "show", "--no-renames", "--no-color", "--format=", sha, "--", path)
	if err != nil {
		return "", err
	}
	return truncate(strings.TrimSpace(out), 800), nil
}

func (g *GitHistory) headSHA(ctx context.Context) (string, error) {
	out, err := g.run(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// run ejecuta git con ctx timeout y devuelve stdout. Usar exec.CommandContext
// garantiza que un git colgado no bloquee el handler.
func (g *GitHistory) run(ctx context.Context, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.RepoPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

// parseVersion convierte una línea del formato numstat-like (%x1f)
// en una Version. Devuelve ok=false si la línea no tiene los 7
// campos esperados (líneas vacías, formato custom, etc.).
func parseVersion(line string) (Version, bool) {
	parts := strings.Split(line, "\x1f")
	if len(parts) < 7 {
		return Version{}, false
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(parts[5]), 10, 64)
	if err != nil {
		return Version{}, false
	}
	return Version{
		SHA:         strings.TrimSpace(parts[0]),
		ShortSHA:    strings.TrimSpace(parts[1]),
		Message:     strings.TrimSpace(parts[2]),
		Author:      strings.TrimSpace(parts[3]),
		AuthorEmail: strings.TrimSpace(parts[4]),
		When:        time.Unix(ts, 0),
		Branch:      strings.TrimSpace(parts[6]),
	}, true
}

// parseNumstat convierte la salida de `git diff --numstat` en una
// lista de VersionFile. Líneas con "-" en adds/dels indican archivos
// binarios o renames que no podemos contabilizar bien; los marcamos
// como Binary=true y dejamos adds/dels en 0.
func parseNumstat(out string) []VersionFile {
	lines := splitLines(out)
	result := make([]VersionFile, 0, len(lines))
	for _, line := range lines {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 {
			continue
		}
		bin := fields[0] == "-" && fields[1] == "-"
		adds, _ := strconv.Atoi(fields[0])
		dels, _ := strconv.Atoi(fields[1])
		result = append(result, VersionFile{
			Path:   strings.TrimSpace(fields[2]),
			Adds:   adds,
			Dels:   dels,
			Binary: bin,
		})
	}
	return result
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(strings.TrimSpace(s), "\n")
}

// extractBranchFromMessage busca el nombre de branch en el subject
// de un merge commit. Soporta las formas que git genera por defecto:
//   - Merge branch 'feature/x' into main
//   - Merge branch 'feature/x'
//   - Merge tag 'v1.0' into main
// Si no matchea, devuelve "" (mejor mostrar sin branch que inventar).
func extractBranchFromMessage(msg string) string {
	msg = firstLine(msg)
	// `Merge branch 'NAME'...`
	const branchKey = "Merge branch '"
	if i := strings.Index(msg, branchKey); i >= 0 {
		rest := msg[i+len(branchKey):]
		if j := strings.Index(rest, "'"); j >= 0 {
			return rest[:j]
		}
	}
	// `Merge tag 'NAME'...`
	const tagKey = "Merge tag '"
	if i := strings.Index(msg, tagKey); i >= 0 {
		rest := msg[i+len(tagKey):]
		if j := strings.Index(rest, "'"); j >= 0 {
			return "tag:" + rest[:j]
		}
	}
	return ""
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}