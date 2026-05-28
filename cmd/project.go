package cmd

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"einarc/internal/api"
	"einarc/internal/config"

	"github.com/spf13/cobra"
)

var (
	skipMutagenCheck       bool
	initMutagenDestination string
	initMutagenName        string
	skipMutagenStart       bool
	skipSSHOnboarding      bool
	skipAirAutoStart       bool
	skipInitialCommit      bool
	forceWorkspaceTakeover bool
)

var initCmd = &cobra.Command{
	Use:   "init <name>",
	Short: "Inicializa un proyecto en Einar",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		if name == "" {
			return fmt.Errorf("nombre de proyecto requerido")
		}

		cfg, err := config.Resolve(apiURLFlag, tokenFlag)
		if err != nil {
			return err
		}
		if cfg.Token == "" {
			return fmt.Errorf("falta token (usa 'login' o EINAR_TOKEN)")
		}

		if debugHTTP {
			fmt.Println("[debug] HTTP request")
			fmt.Printf("[debug] POST %s/api/projects\n", cfg.APIURL)
			fmt.Printf("[debug] Authorization: Bearer %s\n", config.MaskToken(cfg.Token))
			fmt.Printf("[debug] Body: {\"name\":%q,\"public\":true,\"visibility\":\"public\"}\n", name)
		}

		client := api.NewClient(cfg.APIURL, cfg.Token, 10*time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		resp, err := client.CreateProject(ctx, name)
		if err != nil {
			if refreshed, rerr := shouldRefreshAndRetry(err, &cfg); rerr != nil {
				return rerr
			} else if refreshed {
				client = api.NewClient(cfg.APIURL, cfg.Token, 10*time.Second)
				resp, err = client.CreateProject(ctx, name)
			}
		}
		if err != nil {
			if debugHTTP {
				if ae := (&api.APIError{}); api.AsAPIError(err, ae) {
					fmt.Printf("[debug] HTTP error status=%d code=%q message=%q\n", ae.StatusCode, ae.Code, ae.Message)
					if strings.TrimSpace(ae.RawBody) != "" {
						fmt.Printf("[debug] HTTP error body: %s\n", ae.RawBody)
					}
				} else {
					fmt.Printf("[debug] HTTP transport error: %v\n", err)
				}
			}
			if msg := mapAPIError(err); msg != "" {
				return fmt.Errorf("%s", msg)
			}
			return err
		}
		if debugHTTP {
			if b, merr := json.MarshalIndent(resp, "", "  "); merr == nil {
				fmt.Printf("[debug] Response body:\n%s\n", string(b))
			}
		}
		projectID := strings.TrimSpace(resp.ProjectID)
		slug := strings.TrimSpace(resp.Slug)
		if projectID == "" || slug == "" {
			return fmt.Errorf("respuesta inválida del backend canonical: projectId/slug vacíos")
		}

		localProjectDir := slug
		if localProjectDir == "" {
			localProjectDir = strings.TrimSpace(name)
		}
		if err := os.MkdirAll(localProjectDir, 0o755); err != nil {
			return fmt.Errorf("no se pudo crear carpeta local del proyecto: %w", err)
		}
		if err := os.Chdir(localProjectDir); err != nil {
			return fmt.Errorf("no se pudo entrar a la carpeta local del proyecto: %w", err)
		}

		if err := ensureGoProjectScaffold(slug); err != nil {
			return fmt.Errorf("no se pudo preparar scaffold Go base: %w", err)
		}
		if err := ensureProjectGitRepository(); err != nil {
			return fmt.Errorf("no se pudo inicializar repositorio git del proyecto: %w", err)
		}
		if err := ensureLocalAirConfig(); err != nil {
			return fmt.Errorf("no se pudo preparar .air.toml base: %w", err)
		}
		if err := ensureWorkspaceConfig(slug); err != nil {
			return fmt.Errorf("no se pudo preparar workspaces.yaml base: %w", err)
		}
		if err := ensureProjectGitIgnore(); err != nil {
			return fmt.Errorf("no se pudo preparar .gitignore base: %w", err)
		}
		if err := ensureGitHooksForWorkspaceLock(); err != nil {
			return fmt.Errorf("no se pudo preparar hooks git de workspace lock: %w", err)
		}
		if err := ensureInitialGitCommit(skipInitialCommit); err != nil {
			return fmt.Errorf("no se pudo crear commit inicial del proyecto: %w", err)
		}
		if currentBranch, ok := currentGitBranch(); ok {
			cfg.WorkspaceBranch = currentBranch
		} else if strings.TrimSpace(cfg.WorkspaceBranch) == "" {
			cfg.WorkspaceBranch = "main"
		}
		fmt.Println("ℹ️  Workspaces base configurados en workspaces.yaml (prod/main y develop/develop)")
		if isGitRepository() && !gitBranchExists("develop") {
			fmt.Println("ℹ️  Recomendación: crea la branch 'develop' si usarás flujo GitFlow")
		}

		cfg.LastProjectID = projectID
		cfg.LastProjectSlug = slug
		cfg.MutagenDestination = strings.TrimSpace(resp.Sync.Destination)
		cfg.MutagenSessionName = strings.TrimSpace(resp.Sync.SessionName)
		cfg.LastVMName = strings.TrimSpace(resp.VM.Name)
		cfg.LastVMHTTPSURL = strings.TrimSpace(resp.VM.HTTPSURL)
		cfg.LastVMSshDest = strings.TrimSpace(resp.VM.SSHDestination)
		cfg.WorkspaceBranch = strings.TrimSpace(resp.Workspace.Branch)
		if strings.TrimSpace(resp.ProjectAPIToken) != "" {
			cfg.ProjectAPIToken = strings.TrimSpace(resp.ProjectAPIToken)
		}
		cfg.ProjectDBName = strings.TrimSpace(resp.Database.Name)
		cfg.ProjectDBUser = strings.TrimSpace(resp.Database.User)
		cfg.ProjectDBHost = strings.TrimSpace(resp.Database.Host)
		if resp.Database.Port > 0 {
			cfg.ProjectDBPort = resp.Database.Port
		}
		if strings.TrimSpace(resp.VMSshPrivateKey) != "" {
			if err := writeSSHPrivateKey(resp.VMSshPrivateKey); err != nil {
				return fmt.Errorf("no se pudo guardar clave SSH privada: %w", err)
			}
		}
		if strings.TrimSpace(cfg.MutagenDestination) == "" {
			return fmt.Errorf("respuesta inválida del backend canonical: sync.destination vacío")
		}
		if strings.TrimSpace(cfg.MutagenSessionName) == "" {
			return fmt.Errorf("respuesta inválida del backend canonical: sync.sessionName vacío")
		}
		if strings.TrimSpace(cfg.LastVMSshDest) == "" {
			return fmt.Errorf("respuesta inválida del backend canonical: vm.sshDestination vacío")
		}
		if strings.TrimSpace(cfg.WorkspaceBranch) == "" {
			return fmt.Errorf("respuesta inválida del backend canonical: workspace.branch vacío")
		}

		if err := ensureWorkspaceOwnership(&cfg, forceWorkspaceTakeover); err != nil {
			return err
		}
		if err := saveProjectConfig(cfg); err != nil {
			return fmt.Errorf("no se pudo guardar config local: %w", err)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(resp)
		}

		fmt.Println("✅ Proyecto creado")
		fmt.Printf("ID: %s\n", projectID)
		fmt.Printf("Name: %s\n", resp.Name)
		fmt.Printf("Slug: %s\n", slug)
		fmt.Printf("Subdomain: %s\n", resp.Subdomain)
		fmt.Printf("Status: %s\n", resp.Status)
		if wd, werr := os.Getwd(); werr == nil {
			fmt.Printf("Local folder: %s\n", wd)
		}
		if strings.TrimSpace(cfg.LastVMName) != "" {
			fmt.Printf("VM: %s\n", cfg.LastVMName)
		}
		if strings.TrimSpace(cfg.LastVMHTTPSURL) != "" {
			fmt.Printf("VM URL: %s\n", cfg.LastVMHTTPSURL)
		}
		if strings.TrimSpace(cfg.LastVMSshDest) != "" {
			fmt.Printf("VM SSH: %s\n", cfg.LastVMSshDest)
		}
		if strings.TrimSpace(cfg.MutagenDestination) != "" {
			fmt.Printf("Mutagen destination: %s\n", cfg.MutagenDestination)
		}
		if strings.TrimSpace(cfg.ProjectDBName) != "" {
			fmt.Println("DB credentials:")
			fmt.Printf("  dbName: %s\n", cfg.ProjectDBName)
			fmt.Printf("  dbUser: %s\n", cfg.ProjectDBUser)
			fmt.Printf("  dbPassword: %s\n", cfg.ProjectDBPassword)
			fmt.Printf("  dbHost: %s\n", cfg.ProjectDBHost)
			if cfg.ProjectDBPort > 0 {
				fmt.Printf("  dbPort: %d\n", cfg.ProjectDBPort)
			}
		}

		airStarted := false
		mutagenReady := false
		httpChecked := false
		httpReady := false
		httpCode := 0
		var airErr error

		if !skipMutagenCheck {
			if err := ensureMutagenOnWindows(); err != nil {
				fmt.Printf("⚠️  Proyecto creado, pero no se pudo verificar/instalar Mutagen: %v\n", err)
				fmt.Println("   Puedes instalarlo manualmente y luego correr: einarc mutagen ...")
			} else {
				fmt.Println("✅ Mutagen disponible en esta máquina")
				if err := setupAndStartMutagen(&cfg); err != nil {
					fmt.Printf("⚠️  Mutagen disponible, pero no se pudo auto-configurar/start: %v\n", err)
					fmt.Printf("   Puedes correr manualmente: %s daemon start && %s project start\n", mutagenHintCommand(), mutagenHintCommand())
				} else {
					mutagenReady = true
					if !skipAirAutoStart {
						fmt.Println("🚀 Iniciando Air (mandatorio)...")
						maxAirAttempts := 6
						for i := 1; i <= maxAirAttempts; i++ {
							if i == 1 {
								fmt.Println("ℹ️  Preparando/verificando Air en VM...")
							}
							if err := setupAndStartRemoteAir(&cfg); err != nil {
								airErr = err
								errMsg := strings.ToLower(err.Error())
								if strings.Contains(errMsg, "downloading") || strings.Contains(errMsg, "timeout") {
									fmt.Printf("ℹ️  Air intento %d/%d: aún preparando dependencias (%v)\n", i, maxAirAttempts, err)
								} else {
									fmt.Printf("⚠️  Air intento %d/%d: %v\n", i, maxAirAttempts, err)
								}
								time.Sleep(time.Duration(i) * 2 * time.Second)
								continue
							}
							airStarted = true
							fmt.Println("✅ Air iniciado")
							break
						}
						if !airStarted {
							fmt.Println("❌ No se pudo iniciar Air automáticamente tras varios intentos")
							fmt.Println("   Revisa logs con: einarc dev logs -f")
						}
					}
				}
			}
		}

		if strings.TrimSpace(cfg.LastVMHTTPSURL) != "" {
			httpChecked = true
			if airStarted {
				fmt.Printf("🌐 Verificando endpoint HTTP: %s\n", strings.TrimSpace(cfg.LastVMHTTPSURL))
				if code, err := waitForHTTPReady(strings.TrimSpace(cfg.LastVMHTTPSURL), 20*time.Second); err != nil {
					if code > 0 {
						httpCode = code
					}
					fmt.Printf("⚠️  HTTP aún no listo: %v\n", err)
					fmt.Println("   Puedes revisar con: einarc dev logs -f")
				} else {
					httpReady = true
					httpCode = code
					fmt.Printf("✅ App lista (%d): %s\n", code, strings.TrimSpace(cfg.LastVMHTTPSURL))
				}
			} else {
				fmt.Printf("⚠️  HTTP no verificable porque Air no inició: %s\n", strings.TrimSpace(cfg.LastVMHTTPSURL))
			}
		}

		fmt.Println("\n📌 Resumen final")
		if mutagenReady {
			fmt.Println("   - Sync: ✅")
		} else if skipMutagenCheck {
			fmt.Println("   - Sync: ⏭️  omitido (--skip-mutagen-check)")
		} else {
			fmt.Println("   - Sync: ⚠️")
		}
		if airStarted {
			fmt.Println("   - Air: ✅")
		} else if skipAirAutoStart {
			fmt.Println("   - Air: ⏭️  omitido (--skip-air-auto-start)")
		} else {
			fmt.Println("   - Air: ⚠️")
		}
		if !httpChecked {
			fmt.Println("   - HTTP: ⏭️  sin URL")
		} else if httpReady {
			fmt.Printf("   - HTTP: ✅ (%d)\n", httpCode)
		} else if httpCode > 0 {
			fmt.Printf("   - HTTP: ⚠️  (%d)\n", httpCode)
		} else {
			fmt.Println("   - HTTP: ⚠️")
		}
		if strings.TrimSpace(cfg.LastVMHTTPSURL) != "" {
			fmt.Printf("   - URL: %s\n", strings.TrimSpace(cfg.LastVMHTTPSURL))
		}
		if !airStarted || !httpReady {
			fmt.Println("   - Siguiente paso: einarc dev logs -f")
		}
		if !skipAirAutoStart && !airStarted {
			if !mutagenReady {
				fmt.Println("⚠️  Init completado con advertencias: no hubo conectividad SSH al VM, Air no pudo iniciar automáticamente.")
				fmt.Println("   Cuando el VM esté accesible por SSH, ejecuta: einarc dev logs -f")
				return nil
			}
			if airErr != nil {
				return fmt.Errorf("init incompleto: Air es mandatorio y no inició (%v)", airErr)
			}
			return fmt.Errorf("init incompleto: Air es mandatorio y no inició")
		}
		return nil
	},
}

func shouldRefreshAndRetry(err error, cfg *config.Config) (bool, error) {
	var ae api.APIError
	if !api.AsAPIError(err, &ae) || ae.StatusCode != 401 {
		return false, nil
	}
	ok, rerr := tryRefreshAndSave(cfg)
	if rerr != nil {
		return false, rerr
	}
	return ok, nil
}

func mapAPIError(err error) string {
	var ae api.APIError
	if !api.AsAPIError(err, &ae) {
		return ""
	}

	switch ae.StatusCode {
	case 401:
		return "Token inválido/revocado/expirado. Ejecuta login nuevamente."
	case 403:
		if strings.Contains(strings.ToLower(ae.Code), "scope") || strings.Contains(strings.ToLower(ae.Message), "scope") {
			return "Tu token no incluye scope projects:create."
		}
		return "No tienes permisos suficientes (owner/admin)."
	case 409:
		return "Ya existe un proyecto con ese nombre/slug en este tenant."
	case 422:
		return "El nombre no permite generar un slug válido. Intenta otro nombre."
	default:
		return fmt.Sprintf("Error API (%d): %s", ae.StatusCode, ae.Message)
	}
}

func ensureGoProjectScaffold(slug string) error {
	s := strings.TrimSpace(slug)
	if s == "" {
		s = "app"
	}

	if _, err := os.Stat("go.mod"); os.IsNotExist(err) {
		goMod := fmt.Sprintf("module %s\n\ngo 1.22\n", s)
		if werr := os.WriteFile("go.mod", []byte(goMod), 0o644); werr != nil {
			return werr
		}
		fmt.Println("✅ go.mod generado")
	}

	if _, err := os.Stat("main.go"); os.IsNotExist(err) {
		mainGo := `package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok: %s\\n", r.URL.Path)
	})

	addr := ":" + port
	log.Printf("server listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
`
		if werr := os.WriteFile("main.go", []byte(mainGo), 0o644); werr != nil {
			return werr
		}
		fmt.Println("✅ main.go generado")
	}

	return nil
}

func airTomlContent() string {
	return `root = "."

tmp_dir = "tmp"

[build]
  cmd = "go build -o ./tmp/main ."
  bin = "./tmp/main"
  include_ext = ["go", "tpl", "tmpl", "html"]
  exclude_dir = ["assets", "tmp", "vendor", "node_modules", ".git", ".einar"]
  delay = 500

[log]
  time = true
`
}

func ensureLocalAirConfig() error {
	if _, err := os.Stat(".air.toml"); err == nil {
		return nil
	}
	if err := os.WriteFile(".air.toml", []byte(airTomlContent()), 0o644); err != nil {
		return err
	}
	fmt.Println("✅ .air.toml generado")
	return nil
}

func workspaceYAMLContent(slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		slug = "unknown-project"
	}
	return fmt.Sprintf(`version: 1

project:
  slug: %s

defaults:
  baseBranch: main
  portStart: 3000
  portStrategy: sequential # sequential | hash
  runtime: air
  sync: mutagen
  previewDomain: project.dev

templates:
  prod:
    branch: main
    port: 3000
    persistent: true
    runtime: air

  develop:
    branch: develop
    port: 3001
    persistent: true
    runtime: air

rules:
  - pattern: "feature/*"
    persistent: false
    runtime: air

  - pattern: "issue/*"
    persistent: false
    runtime: air

  - pattern: "agent/*"
    persistent: false
    runtime: air
`, slug)
}

func ensureWorkspaceConfig(slug string) error {
	if _, err := os.Stat("workspaces.yaml"); err == nil {
		return nil
	}
	if err := os.WriteFile("workspaces.yaml", []byte(workspaceYAMLContent(slug)), 0o644); err != nil {
		return err
	}
	fmt.Println("✅ workspaces.yaml generado")
	return nil
}

func ensureProjectGitIgnore() error {
	const path = ".gitignore"
	required := []string{
		".einar/",
		"tmp/",
		".air.log",
		".air.pid",
		"mutagen.yml.lock",
	}

	existing := ""
	if b, err := os.ReadFile(path); err == nil {
		existing = string(b)
	} else if !os.IsNotExist(err) {
		return err
	}

	toAdd := make([]string, 0, len(required))
	for _, rule := range required {
		if !strings.Contains(existing, rule) {
			toAdd = append(toAdd, rule)
		}
	}
	if len(toAdd) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString(existing)
	if strings.TrimSpace(existing) != "" && !strings.HasSuffix(existing, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString("\n# Einar local runtime files\n")
	for _, rule := range toAdd {
		sb.WriteString(rule)
		sb.WriteString("\n")
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	fmt.Println("✅ .gitignore actualizado")
	return nil
}

func ensureGitHooksForWorkspaceLock() error {
	if !isGitRepository() {
		return nil
	}
	if err := os.MkdirAll(".githooks", 0o755); err != nil {
		return err
	}

	hookPath := filepath.Join(".githooks", "post-checkout")
	hookContent := `#!/usr/bin/env sh
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

	if err := os.WriteFile(hookPath, []byte(hookContent), 0o755); err != nil {
		return err
	}

	cmd := exec.Command("git", "config", "core.hooksPath", ".githooks")
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("git config core.hooksPath falló: %s", msg)
		}
		return err
	}

	fmt.Println("✅ Hook git instalado para bloquear checkout/switch en workspace")
	return nil
}

func isGitRepository() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

func ensureProjectGitRepository() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	wd = filepath.Clean(wd)

	if isGitRepository() {
		cmd := exec.Command("git", "rev-parse", "--show-toplevel")
		out, err := cmd.CombinedOutput()
		if err == nil {
			top := filepath.Clean(strings.TrimSpace(string(out)))
			if top == wd {
				return nil
			}
		}
		// Está dentro de otro repo: creamos repo propio anidado para aislar workspace.
	}

	init := exec.Command("git", "init", "-b", "main")
	if out, err := init.CombinedOutput(); err != nil {
		// Fallback para versiones viejas de git sin -b
		fallback := exec.Command("git", "init")
		if fbOut, fbErr := fallback.CombinedOutput(); fbErr != nil {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = strings.TrimSpace(string(fbOut))
			}
			if msg != "" {
				return fmt.Errorf("git init falló: %s", msg)
			}
			return fbErr
		}
		checkout := exec.Command("git", "checkout", "-b", "main")
		_ = checkout.Run()
		fmt.Println("✅ Repositorio Git inicializado en main (fallback)")
		return nil
	}

	fmt.Println("✅ Repositorio Git inicializado en main")
	return nil
}

func currentGitBranch() (string, bool) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", false
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return "", false
	}
	return branch, true
}

func ensureInitialGitCommit(skip bool) error {
	if skip || !isGitRepository() {
		return nil
	}

	if err := exec.Command("git", "rev-parse", "--verify", "HEAD").Run(); err == nil {
		return nil
	}

	add := exec.Command("git", "add", ".")
	if out, err := add.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("git add falló: %s", msg)
		}
		return err
	}

	commit := exec.Command("git", "commit", "-m", "chore: initial scaffold by einarc")
	if out, err := commit.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(strings.ToLower(msg), "author identity unknown") || strings.Contains(strings.ToLower(msg), "please tell me who you are") {
			return fmt.Errorf("git commit requiere user.name/user.email configurados")
		}
		if msg != "" {
			return fmt.Errorf("git commit inicial falló: %s", msg)
		}
		return err
	}

	fmt.Println("✅ Commit inicial Git creado")
	return nil
}

func ensureWorkspaceBranchLock(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	currentBranch, ok := currentGitBranch()
	if !ok {
		return nil
	}
	locked := strings.TrimSpace(cfg.WorkspaceBranch)
	if locked == "" {
		cfg.WorkspaceBranch = currentBranch
		if err := saveProjectConfig(*cfg); err != nil {
			return fmt.Errorf("no se pudo persistir workspaceBranch: %w", err)
		}
		return nil
	}
	if locked != currentBranch {
		return fmt.Errorf("branch inválida para este workspace: esperada=%q actual=%q. Evita git switch/checkout en este workspace", locked, currentBranch)
	}
	return nil
}

type workspaceStateEntry struct {
	ProjectSlug         string `json:"projectSlug"`
	WorkspaceBranch     string `json:"workspaceBranch"`
	MutagenDestination  string `json:"mutagenDestination,omitempty"`
	MutagenSessionName  string `json:"mutagenSessionName,omitempty"`
	OwnerID             string `json:"ownerId"`
	UpdatedAt           string `json:"updatedAt"`
}

type workspaceStateFile struct {
	Entries []workspaceStateEntry `json:"entries"`
}

func workspaceStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".einar", "workspaces-state.json"), nil
}

func currentWorkspaceOwnerID() string {
	host, _ := os.Hostname()
	user := os.Getenv("USERNAME")
	if strings.TrimSpace(user) == "" {
		user = os.Getenv("USER")
	}
	if strings.TrimSpace(host) == "" {
		host = "unknown-host"
	}
	if strings.TrimSpace(user) == "" {
		user = "unknown-user"
	}
	return user + "@" + host
}

func loadWorkspaceState() (workspaceStateFile, string, error) {
	path, err := workspaceStatePath()
	if err != nil {
		return workspaceStateFile{}, "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return workspaceStateFile{}, path, nil
		}
		return workspaceStateFile{}, path, err
	}
	var st workspaceStateFile
	if len(strings.TrimSpace(string(b))) == 0 {
		return workspaceStateFile{}, path, nil
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return workspaceStateFile{}, path, err
	}
	return st, path, nil
}

func saveWorkspaceState(path string, st workspaceStateFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func ensureWorkspaceOwnership(cfg *config.Config, forceTakeover bool) error {
	if cfg == nil {
		return nil
	}
	project := strings.TrimSpace(cfg.LastProjectSlug)
	branch := strings.TrimSpace(cfg.WorkspaceBranch)
	if project == "" || branch == "" {
		return nil
	}
	owner := currentWorkspaceOwnerID()
	state, statePath, err := loadWorkspaceState()
	if err != nil {
		return fmt.Errorf("no se pudo leer estado global de workspaces: %w", err)
	}

	destination := strings.TrimSpace(cfg.MutagenDestination)
	session := strings.TrimSpace(cfg.MutagenSessionName)
	idx := -1
	for i := range state.Entries {
		e := state.Entries[i]
		if e.ProjectSlug == project && e.WorkspaceBranch == branch {
			idx = i
			continue
		}
		if destination != "" && strings.EqualFold(strings.TrimSpace(e.MutagenDestination), destination) {
			if !forceTakeover {
				return fmt.Errorf("workspace en uso: mutagen destination ya reservado por %s (%s/%s). Usa --force-workspace-takeover", e.OwnerID, e.ProjectSlug, e.WorkspaceBranch)
			}
		}
		if session != "" && strings.EqualFold(strings.TrimSpace(e.MutagenSessionName), session) {
			if !forceTakeover {
				return fmt.Errorf("workspace en uso: mutagen session ya reservada por %s (%s/%s). Usa --force-workspace-takeover", e.OwnerID, e.ProjectSlug, e.WorkspaceBranch)
			}
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	entry := workspaceStateEntry{
		ProjectSlug:        project,
		WorkspaceBranch:    branch,
		MutagenDestination: destination,
		MutagenSessionName: session,
		OwnerID:            owner,
		UpdatedAt:          now,
	}
	if idx >= 0 {
		existing := state.Entries[idx]
		if strings.TrimSpace(existing.OwnerID) != "" && existing.OwnerID != owner && !forceTakeover {
			return fmt.Errorf("workspace %s/%s está siendo usado por %s. Usa --force-workspace-takeover para tomar control", project, branch, existing.OwnerID)
		}
		state.Entries[idx] = entry
	} else {
		state.Entries = append(state.Entries, entry)
	}

	if err := saveWorkspaceState(statePath, state); err != nil {
		return fmt.Errorf("no se pudo guardar estado global de workspaces: %w", err)
	}
	return nil
}

func gitBranchExists(branch string) bool {
	b := strings.TrimSpace(branch)
	if b == "" {
		return false
	}
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+b)
	if err := cmd.Run(); err == nil {
		return true
	}
	cmd = exec.Command("git", "ls-remote", "--heads", "origin", b)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

func setupAndStartRemoteAir(cfg *config.Config) error {
	if err := ensureWorkspaceBranchLock(cfg); err != nil {
		return err
	}
	if err := ensureWorkspaceOwnership(cfg, false); err != nil {
		return err
	}
	if err := ensureLocalAirConfig(); err != nil {
		return err
	}
	destination := strings.TrimSpace(initMutagenDestination)
	if destination == "" {
		destination = strings.TrimSpace(cfg.MutagenDestination)
	}
	destination = normalizeMutagenDestinationForProject(destination)
	target, remotePath, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		return fmt.Errorf("no se pudo resolver destino remoto para air")
	}

	installCmd := `if ! command -v go >/dev/null 2>&1; then echo "missing go"; exit 21; fi; if ! command -v air >/dev/null 2>&1 && [ ! -x "$HOME/go/bin/air" ]; then go install github.com/air-verse/air@latest; fi; if command -v air >/dev/null 2>&1; then echo "air ready: $(command -v air)"; elif [ -x "$HOME/go/bin/air" ]; then echo "air ready: $HOME/go/bin/air"; else echo "missing air after install"; exit 22; fi`
	msg, err := runSSHScriptWithTimeout(target, installCmd, 4*time.Minute)
	if err != nil {
		if msg != "" {
			if strings.Contains(msg, "missing go") {
				return fmt.Errorf("la VM no tiene Go instalado; instala Go y reintenta")
			}
			return fmt.Errorf("falló instalación/verificación de air: %s", msg)
		}
		return fmt.Errorf("falló instalación/verificación de air: %w", err)
	}

	remoteEnsureAirToml := fmt.Sprintf("cd %q && if [ ! -f .air.toml ]; then cat > .air.toml <<'EOF'\n%s\nEOF\n echo 'generated .air.toml remotely'; fi", remotePath, airTomlContent())
	msg, err = runSSHScriptWithTimeout(target, remoteEnsureAirToml, 45*time.Second)
	if err != nil {
		if msg != "" {
			return fmt.Errorf("falló preparación remota de .air.toml: %s", msg)
		}
		return fmt.Errorf("falló preparación remota de .air.toml: %w", err)
	}

	startCmd := fmt.Sprintf(`cd %q && mkdir -p tmp && AIR_BIN="$(command -v air || echo $HOME/go/bin/air)" && if [ ! -f .air.toml ]; then echo "missing .air.toml"; exit 12; fi && if [ -f .air.pid ] && kill -0 "$(cat .air.pid)" 2>/dev/null; then echo "air ya corriendo"; exit 0; fi && nohup "$AIR_BIN" -c .air.toml > .air.log 2>&1 & echo $! > .air.pid && sleep 1 && if [ -f .air.pid ] && kill -0 "$(cat .air.pid)" 2>/dev/null; then echo "air started pid=$(cat .air.pid)"; else echo "air exited"; tail -n 120 .air.log 2>/dev/null || true; exit 13; fi`, remotePath)
	msg, err = runSSHScriptWithTimeout(target, startCmd, 60*time.Second)
	if err != nil {
		low := strings.ToLower(msg)
		if strings.Contains(low, "air started pid=") || strings.Contains(low, "air ya corriendo") {
			fmt.Printf("ℹ️  Air reportó estado saludable: %s\n", msg)
			fmt.Printf("✅ Air iniciado automáticamente en %s:%s\n", target, remotePath)
			return nil
		}
		if msg != "" {
			return fmt.Errorf("falló inicio de air remoto: %s", msg)
		}
		return fmt.Errorf("falló inicio de air remoto: %w", err)
	}
	fmt.Printf("✅ Air iniciado automáticamente en %s:%s\n", target, remotePath)
	return nil
}

func ensureMutagenOnWindows() error {
	if _, err := exec.LookPath("mutagen"); err == nil {
		return nil
	}
	if p, _ := localMutagenPath(); p != "" {
		if _, err := os.Stat(p); err == nil {
			return nil
		}
	}

	if runtime.GOOS == "windows" {
		// Intento 1: winget
		if _, err := exec.LookPath("winget"); err == nil {
			cmd := exec.Command("winget", "install", "--id", "MutagenIO.Mutagen", "-e", "--accept-package-agreements", "--accept-source-agreements")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			_ = cmd.Run()
			if _, err := exec.LookPath("mutagen"); err == nil {
				return nil
			}
		}

		// Intento 2: choco
		if _, err := exec.LookPath("choco"); err == nil {
			cmd := exec.Command("choco", "install", "mutagen", "-y")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			_ = cmd.Run()
			if _, err := exec.LookPath("mutagen"); err == nil {
				return nil
			}
		}
	}

	// Fallback para cualquier SO: descarga directa del asset correcto (GOOS/GOARCH).
	if err := downloadMutagenLocal(); err == nil {
		return nil
	} else {
		fmt.Printf("⚠️  Fallback descarga Mutagen falló: %v\n", err)
	}

	return fmt.Errorf("mutagen no está en PATH y no se pudo instalar automáticamente")
}

func writeSSHPrivateKey(key string) error {
	path, err := config.ConfigPath()
	if err != nil {
		return err
	}
	keyPath := filepath.Join(filepath.Dir(path), "id_ed25519")
	content := strings.TrimSpace(key) + "\n"
	if err := os.WriteFile(keyPath, []byte(content), 0o600); err != nil {
		return err
	}
	fmt.Printf("✅ SSH private key guardada en %s\n", keyPath)
	return nil
}

func saveProjectConfig(cfg config.Config) error {
	cfg.APIURL = ""
	cfg.Token = ""
	cfg.RefreshToken = ""
	return config.Save(cfg)
}

func setupAndStartMutagen(cfg *config.Config) error {
	if err := ensureWorkspaceBranchLock(cfg); err != nil {
		return err
	}
	if err := ensureWorkspaceOwnership(cfg, false); err != nil {
		return err
	}
	mutagenBin, err := resolveMutagenBinary()
	if err != nil {
		return err
	}

	destination := strings.TrimSpace(initMutagenDestination)
	if destination == "" {
		destination = strings.TrimSpace(cfg.MutagenDestination)
	}
	sessionName := strings.TrimSpace(initMutagenName)
	if sessionName == "" {
		sessionName = strings.TrimSpace(cfg.MutagenSessionName)
	}
	if sessionName == "" {
		sessionName = defaultMutagenSessionName(cfg.LastProjectSlug)
		cfg.MutagenSessionName = sessionName
		_ = saveProjectConfig(*cfg)
	}

	if _, err := os.Stat("mutagen.yml"); os.IsNotExist(err) {
		if destination == "" {
			return fmt.Errorf("falta destino mutagen (usa --mutagen-destination o configura PROJECTS_SYNC_SSH_* en backend)")
		}
		destination = normalizeMutagenDestinationForProject(destination)
		fmt.Printf("ℹ️  Mutagen destination normalizado: %s\n", destination)
		alpha := "."
		if runtime.GOOS != "windows" {
			source, err := os.Getwd()
			if err != nil {
				return err
			}
			absSource, err := filepath.Abs(source)
			if err != nil {
				return err
			}
			alpha = absSource
		}
		if !strings.Contains(destination, "://") && !strings.Contains(destination, ":/") {
			return fmt.Errorf("destino mutagen inválido: %q (debe ser ssh://.../ruta, user@host:/ruta o docker://.../ruta)", destination)
		}
		content := fmt.Sprintf(`sync:
  defaults:
    mode: "two-way-resolved"
    ignore:
      vcs: true
      paths:
        - "node_modules"
        - ".git"

  %s:
    alpha: "%s"
    beta: "%s"
`, sessionName, alpha, destination)
		if err := os.WriteFile("mutagen.yml", []byte(content), 0o644); err != nil {
			return err
		}
		fmt.Println("✅ mutagen.yml generado automáticamente")
	}

	if skipMutagenStart {
		fmt.Println("ℹ️  Se omitió 'mutagen project start' por --skip-mutagen-start")
		return nil
	}
	if err := ensureLocalSSHKeySetup(); err != nil {
		fmt.Printf("⚠️  No se pudo preparar clave SSH local automáticamente: %v\n", err)
	}
	if err := ensureSSHTrustInteractive(destination); err != nil {
		return err
	}
	if err := preflightSSHConnection(destination); err != nil {
		return err
	}
	if err := ensureRemoteMutagenRoot(destination); err != nil {
		return err
	}

	// Evita usar un daemon viejo iniciado con otro binario/carpeta de agentes.
	_ = exec.Command(mutagenBin, "daemon", "stop").Run()

	cmd := exec.Command(mutagenBin, "project", "start")
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		lowOut := strings.ToLower(output)
		if strings.Contains(lowOut, "project already running") {
			fmt.Println("✅ Mutagen ya estaba corriendo para este proyecto")
			if err := ensureInitialSyncHealthy(mutagenBin, sessionName, destination); err != nil {
				fmt.Printf("⚠️  Sync aún no está saludable (continuando): %v\n", err)
			}
			return nil
		}
		if strings.Contains(lowOut, "unable to connect to daemon") || strings.Contains(lowOut, "connection timed out") {
			fmt.Println("ℹ️  Reintentando: iniciando daemon Mutagen y relanzando project start...")
			startDaemon := exec.Command(mutagenBin, "daemon", "start")
			if dOut, dErr := startDaemon.CombinedOutput(); dErr != nil {
				daemonMsg := strings.TrimSpace(string(dOut))
				if daemonMsg != "" {
					fmt.Println(daemonMsg)
				}
				if output != "" {
					fmt.Println(output)
				}
				return dErr
			}
			retry := exec.Command(mutagenBin, "project", "start")
			rOut, rErr := retry.CombinedOutput()
			rMsg := strings.TrimSpace(string(rOut))
			if rErr != nil {
				if rMsg != "" {
					fmt.Println(rMsg)
				}
				if output != "" {
					fmt.Println(output)
				}
				return rErr
			}
			if rMsg != "" {
				fmt.Println(rMsg)
			}
			fmt.Println("✅ Mutagen sync iniciado (mutagen project start)")
			if err := ensureInitialSyncHealthy(mutagenBin, sessionName, destination); err != nil {
				fmt.Printf("⚠️  Sync aún no está saludable (continuando): %v\n", err)
			}
			printMutagenPostInitChecklist(destination, sessionName, strings.TrimSpace(cfg.LastVMHTTPSURL))
			return nil
		}
		if output != "" {
			fmt.Println(output)
		}
		return err
	}
	if output != "" {
		fmt.Println(output)
	}
	fmt.Println("✅ Mutagen sync iniciado (mutagen project start)")
	if err := ensureInitialSyncHealthy(mutagenBin, sessionName, destination); err != nil {
		fmt.Printf("⚠️  Sync aún no está saludable (continuando): %v\n", err)
	}
	printMutagenPostInitChecklist(destination, sessionName, strings.TrimSpace(cfg.LastVMHTTPSURL))
	return nil
}

func printMutagenPostInitChecklist(destination, sessionName, vmURL string) {
	target, remotePath, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		fmt.Println("ℹ️  Verificación manual sugerida: mutagen sync list --long")
		return
	}
	if strings.TrimSpace(sessionName) == "" {
		sessionName = "dev-sync"
	}
	hint := mutagenHintCommand()
	fmt.Println("📋 Checklist de verificación")
	fmt.Printf("   - Sesión: %s\n", sessionName)
	fmt.Printf("   - Destino remoto: %s:%s\n", target, remotePath)
	fmt.Printf("   - Estado: ejecuta '%s sync list --long'\n", hint)
	fmt.Printf("   - VM check: ssh %s \"ls -la %s\"\n", target, remotePath)
	if strings.TrimSpace(vmURL) != "" {
		fmt.Printf("   - HTTP check: curl -I %s\n", strings.TrimSpace(vmURL))
	}
}

func mutagenHintCommand() string {
	p, err := localMutagenPath()
	if err == nil && strings.TrimSpace(p) != "" {
		if runtime.GOOS == "windows" {
			return p
		}
		return p
	}
	return "mutagen"
}

func defaultMutagenSessionName(slug string) string {
	s := strings.TrimSpace(slug)
	if s == "" {
		s = "project"
	}
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	if len(s) > 30 {
		s = s[:30]
	}
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "dev-sync-" + s
	}
	return fmt.Sprintf("dev-sync-%s-%s", s, hex.EncodeToString(b))
}

func ensureInitialSyncHealthy(mutagenBin, sessionName, destination string) error {
	fmt.Println("🔄 Verificando sincronización inicial...")
	target, remotePath, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		return fmt.Errorf("no se pudo resolver destino remoto de mutagen")
	}

	maxAttempts := 3
	for i := 1; i <= maxAttempts; i++ {
		flush := exec.Command(mutagenBin, "sync", "flush", sessionName)
		if out, err := flush.CombinedOutput(); err != nil {
			msg := strings.TrimSpace(string(out))
			if msg != "" {
				fmt.Printf("⚠️  flush intento %d/%d: %s\n", i, maxAttempts, msg)
			}
		} else {
			_ = out
		}

		if err := remoteFilesPresent(target, remotePath, []string{"main.go", ".air.toml"}); err == nil {
			fmt.Println("✅ Sync healthy: archivos críticos presentes en VM")
			return nil
		} else {
			fmt.Printf("⚠️  Verificación remota intento %d/%d: %v\n", i, maxAttempts, err)
		}
		time.Sleep(time.Duration(i) * time.Second)
	}

	return fmt.Errorf("sync no quedó saludable tras reintentos (faltan archivos críticos en VM)")
}

func remoteFilesPresent(target, remotePath string, files []string) error {
	names := make([]string, 0, len(files))
	for _, f := range files {
		name := strings.TrimSpace(f)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil
	}

	cleanRoot := strings.TrimSpace(remotePath)
	cleanRoot = strings.TrimSuffix(cleanRoot, "/")
	if cleanRoot == "" {
		cleanRoot = "/"
	}

	checks := make([]string, 0, len(names))
	for _, n := range names {
		abs := cleanRoot + "/" + strings.TrimPrefix(n, "/")
		checks = append(checks, fmt.Sprintf("if [ ! -f %q ]; then echo missing:%q; fi", abs, n))
	}
	cmdText := fmt.Sprintf("%s; ls -la %q", strings.Join(checks, "; "), cleanRoot)
	msg, err := runSSHScript(target, cmdText)
	if err != nil {
		if msg != "" {
			return fmt.Errorf("remote check error (%s): %s", cleanRoot, msg)
		}
		return err
	}

	if strings.Contains(msg, "missing:") {
		return fmt.Errorf("archivos críticos faltantes en VM (%s:%s):\n%s", target, cleanRoot, msg)
	}
	return nil
}

func waitForHTTPReady(url string, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}
	lastErr := ""
	lastCode := 0

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			lastCode = resp.StatusCode
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return resp.StatusCode, nil
			}
			if resp.StatusCode == 401 || resp.StatusCode == 403 {
				return resp.StatusCode, fmt.Errorf("endpoint alcanzable pero protegido por autenticación (HTTP %d)", resp.StatusCode)
			}
			lastErr = fmt.Sprintf("status HTTP %d", resp.StatusCode)
		} else {
			lastErr = err.Error()
		}
		time.Sleep(1 * time.Second)
	}

	if lastCode != 0 {
		return lastCode, fmt.Errorf("timeout esperando 2xx/3xx (último status: %d)", lastCode)
	}
	if strings.TrimSpace(lastErr) == "" {
		lastErr = "sin respuesta"
	}
	return 0, fmt.Errorf("timeout esperando respuesta HTTP (%s)", lastErr)
}

func ensureSSHTrustInteractive(destination string) error {
	target, ok := sshTargetFromMutagenDestination(destination)
	if !ok {
		return nil
	}
	check := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", target, "exit")
	checkOut, checkErr := check.CombinedOutput()
	if checkErr == nil {
		return nil
	}
	checkMsg := strings.ToLower(strings.TrimSpace(string(checkOut)))
	if strings.Contains(checkMsg, "permission denied") || strings.Contains(checkMsg, "ssh keys are required") || strings.Contains(checkMsg, "authentication failed") {
		return nil
	}

	fmt.Printf("🔐 Primera conexión SSH a %s\n", target)
	fmt.Print("¿Confiar en la host key y continuar con la sincronización? (y/N): ")
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" && answer != "s" && answer != "si" {
		return fmt.Errorf("sincronización cancelada por el usuario (host key no confirmada)")
	}

	accept := exec.Command("ssh", "-o", "StrictHostKeyChecking=accept-new", target, "exit")
	acceptOut, acceptErr := accept.CombinedOutput()
	acceptMsg := strings.ToLower(strings.TrimSpace(string(acceptOut)))
	if acceptErr != nil {
		if strings.Contains(acceptMsg, "permanently added") || strings.Contains(acceptMsg, "known hosts") || strings.Contains(acceptMsg, "permission denied") || strings.Contains(acceptMsg, "ssh keys are required") || strings.Contains(acceptMsg, "authentication failed") {
			fmt.Println(strings.TrimSpace(string(acceptOut)))
			fmt.Println("✅ Host SSH confiado y guardado en known_hosts")
			return nil
		}
		if strings.TrimSpace(string(acceptOut)) != "" {
			return fmt.Errorf("no se pudo registrar host key SSH para %s: %s", target, strings.TrimSpace(string(acceptOut)))
		}
		return fmt.Errorf("no se pudo registrar host key SSH para %s: %w", target, acceptErr)
	}
	if strings.TrimSpace(string(acceptOut)) != "" {
		fmt.Println(strings.TrimSpace(string(acceptOut)))
	}
	fmt.Println("✅ Host SSH confiado y guardado en known_hosts")
	return nil
}

func sshTargetFromMutagenDestination(destination string) (string, bool) {
	target, _, ok := sshTargetAndPathFromMutagenDestination(destination)
	return target, ok
}

func sshTargetAndPathFromMutagenDestination(destination string) (string, string, bool) {
	d := strings.TrimSpace(destination)
	if d == "" {
		return "", "", false
	}
	if strings.HasPrefix(strings.ToLower(d), "ssh://") {
		u, err := url.Parse(d)
		if err != nil || u.Host == "" {
			return "", "", false
		}
		host := u.Hostname()
		if host == "" {
			return "", "", false
		}
		user := ""
		if u.User != nil {
			user = u.User.Username()
		}
		remotePath := u.Path
		if strings.TrimSpace(remotePath) == "" {
			remotePath = "/"
		}
		if user != "" {
			return user + "@" + host, remotePath, true
		}
		return host, remotePath, true
	}
	if strings.Contains(d, "@") && strings.Contains(d, ":") {
		parts := strings.SplitN(d, ":", 2)
		target := strings.TrimSpace(parts[0])
		remotePath := strings.TrimSpace(parts[1])
		if target == "" {
			return "", "", false
		}
		if remotePath == "" {
			remotePath = "/"
		}
		return target, remotePath, true
	}
	return "", "", false
}

func preflightSSHConnection(destination string) error {
	target, _, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		return nil
	}
	check := exec.Command("ssh", "-o", "BatchMode=yes", target, "echo", "ok")
	if out, err := check.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("SSH no disponible para %s: %s", target, msg)
		}
		return fmt.Errorf("SSH no disponible para %s: %w", target, err)
	}
	fmt.Printf("✅ SSH operativo: %s\n", target)
	return nil
}

func ensureExeDevSSHOnboarding(destination string) error {
	if skipSSHOnboarding {
		return nil
	}
	target, _, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		return nil
	}
	lowTarget := strings.ToLower(strings.TrimSpace(target))
	if !strings.Contains(lowTarget, ".exe.xyz") && !strings.Contains(lowTarget, "exe.dev") {
		return nil
	}

	required, msg, err := exeDevRegistrationRequired()
	if err != nil && !required {
		return nil
	}
	if !required {
		return nil
	}

	fmt.Println("🔑 Se requiere onboarding SSH en exe.dev. Ejecutando 'ssh exe.dev'...")
	if strings.TrimSpace(msg) != "" {
		fmt.Println(strings.TrimSpace(msg))
	}
	if !isInteractiveTerminal() {
		return fmt.Errorf("falta completar onboarding SSH en exe.dev (modo no interactivo). Ejecuta 'ssh exe.dev' y reintenta")
	}

	cmd := exec.Command("ssh", "exe.dev")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("onboarding SSH de exe.dev no completado: %w", err)
	}

	if requiredAgain, msgAgain, _ := exeDevRegistrationRequired(); requiredAgain {
		if strings.TrimSpace(msgAgain) != "" {
			return fmt.Errorf("onboarding SSH de exe.dev no completado: %s", strings.TrimSpace(msgAgain))
		}
		return fmt.Errorf("onboarding SSH de exe.dev no completado; ejecuta 'ssh exe.dev' y reintenta")
	}
	fmt.Println("✅ Onboarding SSH de exe.dev completado")
	return nil
}

func exeDevRegistrationRequired() (bool, string, error) {
	check := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "exe.dev", "exit")
	out, err := check.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	low := strings.ToLower(msg)
	if strings.Contains(low, "please complete registration by running: ssh exe.dev") {
		return true, msg, nil
	}
	return false, msg, err
}

func isInteractiveTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func ensureLocalSSHKeySetup() error {
	privPath, pubPath, err := localSSHKeyPaths()
	if err != nil {
		return err
	}
	sshDir := filepath.Dir(privPath)
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(sshDir, 0o700)

	if _, err := os.Stat(privPath); os.IsNotExist(err) {
		if _, lookErr := exec.LookPath("ssh-keygen"); lookErr != nil {
			return fmt.Errorf("falta ssh-keygen para generar %s", privPath)
		}
		cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-f", privPath, "-C", "einarc")
		if out, genErr := cmd.CombinedOutput(); genErr != nil {
			msg := strings.TrimSpace(string(out))
			if msg != "" {
				return fmt.Errorf("no se pudo generar clave SSH: %s", msg)
			}
			return fmt.Errorf("no se pudo generar clave SSH: %w", genErr)
		}
		fmt.Printf("✅ Clave SSH ed25519 generada: %s\n", privPath)
	}

	if err := os.Chmod(privPath, 0o600); err != nil {
		return fmt.Errorf("no se pudo ajustar permisos de %s: %w", privPath, err)
	}

	if _, err := os.Stat(pubPath); os.IsNotExist(err) {
		if _, lookErr := exec.LookPath("ssh-keygen"); lookErr != nil {
			return fmt.Errorf("falta ssh-keygen para derivar %s", pubPath)
		}
		cmd := exec.Command("ssh-keygen", "-y", "-f", privPath)
		out, pubErr := cmd.CombinedOutput()
		if pubErr != nil {
			msg := strings.TrimSpace(string(out))
			if msg != "" {
				return fmt.Errorf("no se pudo derivar clave pública: %s", msg)
			}
			return fmt.Errorf("no se pudo derivar clave pública: %w", pubErr)
		}
		pubKey := strings.TrimSpace(string(out))
		if pubKey == "" {
			return fmt.Errorf("ssh-keygen devolvió clave pública vacía")
		}
		if err := os.WriteFile(pubPath, []byte(pubKey+"\n"), 0o644); err != nil {
			return fmt.Errorf("no se pudo escribir %s: %w", pubPath, err)
		}
	}
	if err := os.Chmod(pubPath, 0o644); err != nil {
		return fmt.Errorf("no se pudo ajustar permisos de %s: %w", pubPath, err)
	}

	if strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK")) != "" {
		if _, err := exec.LookPath("ssh-add"); err == nil {
			cmd := exec.Command("ssh-add", privPath)
			_ = cmd.Run()
		}
	}

	fmt.Printf("✅ Permisos SSH local verificados (%s)\n", privPath)
	return nil
}

func localSSHKeyPaths() (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	priv := filepath.Join(home, ".ssh", "id_ed25519")
	pub := priv + ".pub"
	return priv, pub, nil
}

func readLocalSSHPublicKey() (string, string, error) {
	_, pub, err := localSSHKeyPaths()
	if err != nil {
		return "", "", err
	}
	b, err := os.ReadFile(pub)
	if err != nil {
		return pub, "", err
	}
	return pub, strings.TrimSpace(string(b)), nil
}

func ensureRemoteMutagenRoot(destination string) error {
	target, remotePath, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		return nil
	}
	remotePath = strings.TrimSpace(remotePath)
	if remotePath == "" || remotePath == "/" {
		return nil
	}
	msg, err := runSSHScript(target, fmt.Sprintf("mkdir -p %q", remotePath))
	if err != nil {
		if msg != "" {
			return fmt.Errorf("no se pudo crear carpeta remota %s en %s: %s", remotePath, target, msg)
		}
		return fmt.Errorf("no se pudo crear carpeta remota %s en %s: %w", remotePath, target, err)
	}
	fmt.Printf("✅ Carpeta remota lista: %s (%s)\n", remotePath, target)
	return nil
}

func normalizeMutagenDestinationForProject(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return v
	}

	// Soporte ssh://user@host/path -> user@host:/path
	if strings.HasPrefix(strings.ToLower(v), "ssh://") {
		u, err := url.Parse(v)
		if err != nil || u.Host == "" {
			return v
		}
		host := u.Hostname()
		if host == "" {
			return v
		}
		user := ""
		if u.User != nil {
			user = u.User.Username()
		}
		remotePath := u.EscapedPath()
		if remotePath == "" {
			remotePath = "/"
		}
		user, remotePath = normalizeExeDevRemoteTarget(user, remotePath)
		if user != "" {
			return fmt.Sprintf("%s@%s:%s", user, host, remotePath)
		}
		return fmt.Sprintf("%s:%s", host, remotePath)
	}

	// Soporte user@host:/path (SCP-style)
	if strings.Contains(v, "@") && strings.Contains(v, ":") {
		parts := strings.SplitN(v, ":", 2)
		left := strings.TrimSpace(parts[0])
		remotePath := strings.TrimSpace(parts[1])
		if left == "" || remotePath == "" {
			return v
		}
		user := ""
		host := left
		if at := strings.Index(left, "@"); at >= 0 {
			user = strings.TrimSpace(left[:at])
			host = strings.TrimSpace(left[at+1:])
		}
		if host == "" {
			return v
		}
		user, remotePath = normalizeExeDevRemoteTarget(user, remotePath)
		if user != "" {
			return fmt.Sprintf("%s@%s:%s", user, host, remotePath)
		}
		return fmt.Sprintf("%s:%s", host, remotePath)
	}

	return v
}

func normalizeExeDevRemoteTarget(user, remotePath string) (string, string) {
	p := strings.TrimSpace(remotePath)
	if p == "" {
		return user, remotePath
	}
	// Estandarizamos proyectos en /home/exedev/workspace/<slug> cuando backend devuelve /app/projects/<slug>
	if strings.HasPrefix(p, "/app/projects/") {
		slug := strings.TrimPrefix(p, "/app/projects/")
		slug = strings.Trim(slug, "/")
		if slug == "" {
			return "exedev", "/home/exedev/workspace"
		}
		return "exedev", "/home/exedev/workspace/" + slug
	}
	return user, remotePath
}

func resolveMutagenBinary() (string, error) {
	if p, err := exec.LookPath("mutagen"); err == nil {
		return p, nil
	}
	if p, _ := localMutagenPath(); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("mutagen no está disponible")
}

func localMutagenPath() (string, error) {
	binName := "mutagen"
	if runtime.GOOS == "windows" {
		binName = "mutagen.exe"
	}
	// Binario compartido junto al CLI (no por proyecto)
	exePath, err := os.Executable()
	if err == nil && strings.TrimSpace(exePath) != "" {
		exeDir := filepath.Dir(exePath)
		if strings.TrimSpace(exeDir) != "" {
			return filepath.Join(exeDir, ".einar", "bin", binName), nil
		}
	}

	// Fallback defensivo al esquema local si no se puede resolver el ejecutable.
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(cfgPath), "bin", binName), nil
}

func downloadMutagenLocal() error {
	url, err := resolveLatestMutagenAssetURL()
	if err != nil {
		return err
	}
	mutagenPath, err := localMutagenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(mutagenPath), 0o700); err != nil {
		return err
	}
	archiveName := strings.TrimSpace(filepath.Base(strings.Split(url, "?")[0]))
	if archiveName == "" {
		archiveName = "mutagen-archive"
	}
	tmpArchive := filepath.Join(filepath.Dir(mutagenPath), archiveName)

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("descarga mutagen falló: %s", resp.Status)
	}

	f, err := os.Create(tmpArchive)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	f.Close()
	defer os.Remove(tmpArchive)

	baseDir := filepath.Dir(mutagenPath)
	foundExe, foundAgentBundle, err := extractMutagenArchive(tmpArchive, baseDir)
	if err != nil {
		return err
	}

	if !foundExe {
		return fmt.Errorf("no se encontró binario mutagen en el archivo descargado")
	}
	if !foundAgentBundle {
		return fmt.Errorf("no se encontró bundle de agentes en el archivo descargado")
	}

	fmt.Printf("✅ Mutagen descargado en %s\n", mutagenPath)
	return nil
}

func extractMutagenArchive(archivePath, baseDir string) (bool, bool, error) {
	lower := strings.ToLower(strings.TrimSpace(archivePath))
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return extractMutagenZip(archivePath, baseDir)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return extractMutagenTarGz(archivePath, baseDir)
	default:
		return false, false, fmt.Errorf("formato de archivo no soportado: %s", archivePath)
	}
}

func extractMutagenZip(archivePath, baseDir string) (bool, bool, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return false, false, err
	}
	defer zr.Close()

	foundExe := false
	foundAgentBundle := false

	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		name := strings.ToLower(filepath.Base(zf.Name))
		isMutagenBin := name == "mutagen.exe" || name == "mutagen"
		if !isMutagenBin && !strings.Contains(name, "agent") {
			continue
		}

		rc, err := zf.Open()
		if err != nil {
			return false, false, err
		}

		dst := filepath.Join(baseDir, filepath.Base(zf.Name))
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			rc.Close()
			return false, false, err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return false, false, err
		}
		out.Close()
		rc.Close()

		if isMutagenBin {
			foundExe = true
		}
		if strings.Contains(name, "agent") {
			foundAgentBundle = true
		}
	}

	return foundExe, foundAgentBundle, nil
}

func extractMutagenTarGz(archivePath, baseDir string) (bool, bool, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return false, false, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return false, false, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	foundExe := false
	foundAgentBundle := false

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, false, err
		}
		if hdr == nil || hdr.FileInfo().IsDir() {
			continue
		}

		name := strings.ToLower(filepath.Base(hdr.Name))
		isMutagenBin := name == "mutagen.exe" || name == "mutagen"
		if !isMutagenBin && !strings.Contains(name, "agent") {
			continue
		}

		dst := filepath.Join(baseDir, filepath.Base(hdr.Name))
		mode := os.FileMode(0o644)
		if isMutagenBin {
			mode = 0o755
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			return false, false, err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return false, false, err
		}
		if err := out.Close(); err != nil {
			return false, false, err
		}

		if isMutagenBin {
			foundExe = true
		}
		if strings.Contains(name, "agent") {
			foundAgentBundle = true
		}
	}

	return foundExe, foundAgentBundle, nil
}

func resolveLatestMutagenAssetURL() (string, error) {
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/mutagen-io/mutagen/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github latest release request falló: %s body=%s", resp.Status, strings.TrimSpace(string(b)))
	}

	type asset struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	}
	var payload struct {
		Assets []asset `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}

	osName := runtime.GOOS
	arch := runtime.GOARCH
	osAliases := []string{osName}
	if osName == "darwin" {
		osAliases = append(osAliases, "macos", "osx")
	}
	archAliases := []string{arch}
	if arch == "amd64" {
		archAliases = append(archAliases, "x86_64", "x64")
	}
	if arch == "arm64" {
		archAliases = append(archAliases, "aarch64")
	}
	for _, a := range payload.Assets {
		name := strings.ToLower(strings.TrimSpace(a.Name))
		if !strings.HasSuffix(name, ".zip") && !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".tgz") {
			continue
		}
		osMatch := false
		for _, oa := range osAliases {
			if strings.Contains(name, oa) {
				osMatch = true
				break
			}
		}
		if !osMatch {
			continue
		}
		archMatch := false
		for _, aa := range archAliases {
			if strings.Contains(name, aa) {
				archMatch = true
				break
			}
		}
		if !archMatch {
			continue
		}
		if strings.TrimSpace(a.URL) != "" {
			return strings.TrimSpace(a.URL), nil
		}
	}

	return "", fmt.Errorf("no se encontró asset mutagen para %s/%s en latest release", osName, arch)
}

func init() {
	initCmd.Flags().BoolVar(&skipMutagenCheck, "skip-mutagen-check", false, "No verifica/instala Mutagen automáticamente")
	initCmd.Flags().StringVar(&initMutagenDestination, "mutagen-destination", "", "Destino remoto Mutagen (ej: docker://mi-contenedor/var/www)")
	initCmd.Flags().StringVar(&initMutagenName, "mutagen-name", "", "Nombre de sesión en mutagen.yml (por defecto: dev-sync-<slug>-<shortid>)")
	initCmd.Flags().BoolVar(&skipMutagenStart, "skip-mutagen-start", false, "No ejecuta 'mutagen project start'")
	initCmd.Flags().BoolVar(&skipSSHOnboarding, "skip-ssh-onboarding", false, "No ejecuta onboarding automático con 'ssh exe.dev'")
	initCmd.Flags().BoolVar(&skipAirAutoStart, "skip-air-auto-start", false, "No inicia Air automáticamente en la VM")
	initCmd.Flags().BoolVar(&skipInitialCommit, "skip-initial-commit", false, "No crea commit inicial del scaffold del proyecto")
	initCmd.Flags().BoolVar(&forceWorkspaceTakeover, "force-workspace-takeover", false, "Toma control del workspace global aunque otro owner lo tenga reservado")
	rootCmd.AddCommand(initCmd)
}
