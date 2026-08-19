package cmd

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ignaciojeria/sync/internal/config"
)

// ponytail: helpers para que el repo git viva en la VM, no en Windows.
// mutagen.yml ya excluye ".git" → Windows queda como mirror limpio.

// ensureLocalProjectGitRemoved borra un eventual .git local dentro de dir
// (solo si dir es el cwd y existe .git). Valida que dir no escape con "..".
func ensureLocalProjectGitRemoved(dir string) error {
	if strings.Contains(dir, "..") {
		return fmt.Errorf("ruta inválida (contiene '..'): %q", dir)
	}
	dir = filepath.Clean(dir)
	if dir == "" {
		return fmt.Errorf("ruta vacía")
	}
	gitPath := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitPath); os.IsNotExist(err) {
		return nil
	}
	if err := os.RemoveAll(gitPath); err != nil {
		return fmt.Errorf("no se pudo limpiar .git local: %w", err)
	}
	fmt.Println("ℹ️  .git local removido (el repo vive en la VM)")
	return nil
}

// runRemoteGit ejecuta `git <args...>` en la VM vía SSH, dentro del remotePath
// del proyecto. Devuelve stdout combinado y error.
func runRemoteGit(cfg *config.Config, args ...string) (string, error) {
	dest := strings.TrimSpace(cfg.MutagenDestination)
	if dest == "" {
		return "", fmt.Errorf("sin MutagenDestination; no se puede hablar con la VM")
	}
	target, remotePath, ok := sshTargetAndPathFromMutagenDestination(dest)
	if !ok {
		return "", fmt.Errorf("MutagenDestination inválido: %q", dest)
	}
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuote(a)
	}
	script := fmt.Sprintf("cd %s && git %s", shellQuote(remotePath), strings.Join(quoted, " "))
	return runSSHScriptWithTimeout(target, script, 60*time.Second)
}

// ensureRemoteGitInit crea (si no existe) el repo git en la VM y fija
// user.name/user.email locales al repo si no hay config global.
func ensureRemoteGitInit(cfg *config.Config) error {
	dest := strings.TrimSpace(cfg.MutagenDestination)
	if dest == "" {
		return nil
	}
	target, remotePath, ok := sshTargetAndPathFromMutagenDestination(dest)
	if !ok {
		return fmt.Errorf("MutagenDestination inválido: %q", dest)
	}
	// ponytail: el remotePath puede no existir aún; ensureRemoteMutagenRoot
	// corre más tarde dentro de setupAndStartMutagen, así que lo creamos acá.
	// git symbolic-ref --short HEAD devuelve "main" incluso antes del primer
	// commit (rev-parse --abbrev-ref HEAD devuelve "HEAD" en unborn HEAD).
	script := fmt.Sprintf(`set -eu
mkdir -p %s
cd %s
if [ ! -d .git ]; then git init -b main >/dev/null; fi
if [ -z "$(git config --get user.name || true)" ]; then git config user.name "einar"; fi
if [ -z "$(git config --get user.email || true)" ]; then git config user.email "einar@local"; fi
git symbolic-ref --short HEAD
`, shellQuote(remotePath), shellQuote(remotePath))
	out, err := runSSHScriptWithTimeout(target, script, 90*time.Second)
	if err != nil {
		return fmt.Errorf("git init remoto falló en %s (path=%s): %w — salida: %s", target, remotePath, err, strings.TrimSpace(out))
	}
	branch := strings.TrimSpace(out)
	if branch != "" && branch != "HEAD" {
		fmt.Printf("✅ Repositorio Git en VM inicializado en branch %q\n", branch)
	} else {
		fmt.Println("✅ Repositorio Git en VM inicializado")
	}
	return nil
}

// installRemoteGitHook instala el post-checkout hook en la VM para bloquear
// checkout/switch fuera de la workspaceBranch configurada.
func installRemoteGitHook(cfg *config.Config) error {
	dest := strings.TrimSpace(cfg.MutagenDestination)
	if dest == "" {
		return nil
	}
	target, remotePath, ok := sshTargetAndPathFromMutagenDestination(dest)
	if !ok {
		return fmt.Errorf("MutagenDestination inválido: %q", dest)
	}
	const hook = `#!/usr/bin/env sh
set -eu

if [ "${EINAR_HOOK_BYPASS:-}" = "1" ]; then
  exit 0
fi

if [ ! -f ".einar/config.json" ]; then
  exit 0
fi

locked_branch=$(grep -o '"workspaceBranch"[[:space:]]*:[[:space:]]*"[^"]*"' .einar/config.json | sed 's/.*"workspaceBranch"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' || true)
if [ -z "${locked_branch:-}" ]; then
  exit 0
fi

current_branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)
if [ -z "${current_branch:-}" ] || [ "$current_branch" = "HEAD" ]; then
  exit 0
fi

if [ "$current_branch" != "$locked_branch" ]; then
  echo "❌ Checkout bloqueado por Einar workspace lock"
  echo "   workspaceBranch: $locked_branch"
  echo "   branch actual:    $current_branch"
  echo "   Revirtiendo checkout para evitar context switch..."
  EINAR_HOOK_BYPASS=1 git checkout -q "$locked_branch" || {
    echo "⚠️  No se pudo revertir automáticamente. Vuelve manualmente: git checkout $locked_branch"
    exit 1
  }
  echo "✅ Workspace restaurado en '$locked_branch'"
  exit 1
fi

exit 0
`
	b64 := base64.StdEncoding.EncodeToString([]byte(hook))
	script := fmt.Sprintf(`set -eu
mkdir -p %s
cd %s
mkdir -p .githooks
printf '%%s' %s | base64 -d > .githooks/post-checkout
chmod +x .githooks/post-checkout
git config core.hooksPath .githooks
echo "hook-installed"
`, shellQuote(remotePath), shellQuote(remotePath), shellQuote(b64))
	out, err := runSSHScriptWithTimeout(target, script, 60*time.Second)
	if err != nil {
		return fmt.Errorf("instalación de hook remoto falló: %w", err)
	}
	if !strings.Contains(out, "hook-installed") {
		return fmt.Errorf("hook no se instaló correctamente (salida: %s)", strings.TrimSpace(out))
	}
	fmt.Println("✅ Hook git remoto instalado (.githooks/post-checkout)")
	return nil
}

// commitRemoteScaffold hace `git add -A && git commit` en la VM, respetando
// skip. Si no hay nada que commitear (todo committed) no falla.
func commitRemoteScaffold(skip bool, cfg *config.Config) error {
	if skip {
		return nil
	}
	dest := strings.TrimSpace(cfg.MutagenDestination)
	if dest == "" {
		return nil
	}
	target, remotePath, ok := sshTargetAndPathFromMutagenDestination(dest)
	if !ok {
		return fmt.Errorf("MutagenDestination inválido: %q", dest)
	}
	script := fmt.Sprintf(`set -eu
mkdir -p %s
cd %s
git add -A
if git diff --cached --quiet; then
  echo "nothing-to-commit"
  exit 0
fi
git commit -m "chore: initial scaffold by einarc"
`, shellQuote(remotePath), shellQuote(remotePath))
	out, err := runSSHScriptWithTimeout(target, script, 90*time.Second)
	if err != nil {
		return fmt.Errorf("commit inicial remoto falló: %w (salida: %s)", err, strings.TrimSpace(out))
	}
	if strings.Contains(out, "nothing-to-commit") {
		fmt.Println("ℹ️  No hay cambios que commitear en VM (ya había un commit previo)")
		return nil
	}
	fmt.Println("✅ Commit inicial Git creado en VM")
	return nil
}

// getRemoteCurrentBranch devuelve la branch actual del repo en la VM.
// Usa symbolic-ref para tolerar unborn HEAD (sin commits aún).
func getRemoteCurrentBranch(cfg *config.Config) (string, bool) {
	out, err := runRemoteGit(cfg, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", false
	}
	branch := strings.TrimSpace(out)
	if branch == "" || branch == "HEAD" {
		return "", false
	}
	return branch, true
}

// remoteBranchExists chequea si una branch existe en la VM.
func remoteBranchExists(cfg *config.Config, branch string) bool {
	if strings.TrimSpace(branch) == "" {
		return false
	}
	_, err := runRemoteGit(cfg, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// gitOpsAfterMutagen corre git init + hook + commit en la VM y detecta la
// branch activa. Se llama DESPUÉS de setupAndStartMutagen, porque ahí ya está
// el SSH host key trust y el remote directory creado.
//
// ponytail: si algo falla no abortamos el init; la VM queda sin repo y el
// usuario puede arreglarlo con `einarc git init` después. El flujo del
// proyecto (scaffold, sync, Air) ya está sano.
func gitOpsAfterMutagen(cfg *config.Config, skipInitialCommit bool) {
	if strings.TrimSpace(cfg.MutagenDestination) == "" {
		return
	}
	if err := ensureRemoteGitInit(cfg); err != nil {
		fmt.Printf("⚠️  git init en VM falló: %v\n", err)
		return
	}
	if err := installRemoteGitHook(cfg); err != nil {
		fmt.Printf("⚠️  hook git remoto no instalado: %v\n", err)
	}
	if err := commitRemoteScaffold(skipInitialCommit, cfg); err != nil {
		fmt.Printf("⚠️  commit inicial en VM falló: %v\n", err)
	}
	if currentBranch, ok := getRemoteCurrentBranch(cfg); ok {
		cfg.WorkspaceBranch = currentBranch
	} else if strings.TrimSpace(cfg.WorkspaceBranch) == "" {
		cfg.WorkspaceBranch = "main"
	}
	fmt.Println("ℹ️  Workspaces base configurados en workspaces.yaml (prod/main y develop/develop)")
	if !remoteBranchExists(cfg, "develop") {
		fmt.Println("ℹ️  Recomendación: crea la branch 'develop' si usarás flujo GitFlow")
	}
}

