package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ignaciojeria/sync/internal/api"
	"github.com/Ignaciojeria/sync/internal/config"

	"github.com/spf13/cobra"
)

var workspaceSlug string

var takeCmd = &cobra.Command{
	Use:   "take",
	Short: "Trae cambios pendientes y toma ownership del workspace actual",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadWorkspaceConfigForOps()
		if err != nil {
			return err
		}

		fmt.Println("ℹ️  Sincronizando cambios pendientes antes de tomar ownership...")
		if err := runWorkspaceMutagenSync(&cfg); err != nil {
			return err
		}
		if err := ensureWorkspaceOwnership(&cfg, true); err != nil {
			return err
		}
		if err := ensureMutagenSessionHealthy(&cfg); err != nil {
			return fmt.Errorf("ownership transferido pero sync no saludable: %w", err)
		}
		fmt.Println("✅ Ownership transferido al entorno actual")
		return nil
	},
}

func loadWorkspaceConfigForOps() (config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		if !os.IsNotExist(err) {
			return config.Config{}, fmt.Errorf("no se pudo leer config local (.einar/config.json): %w", err)
		}
		cfg, err = bootstrapConfigFromSlug()
		if err != nil {
			return config.Config{}, err
		}
	}

	if strings.TrimSpace(cfg.MutagenDestination) == "" || strings.TrimSpace(cfg.MutagenSessionName) == "" || strings.TrimSpace(cfg.WorkspaceBranch) == "" {
		cfg, err = bootstrapConfigFromSlug()
		if err != nil {
			return config.Config{}, err
		}
	}

	if err := ensureWorkspaceBranchLock(&cfg); err != nil {
		return config.Config{}, err
	}
	if strings.TrimSpace(cfg.MutagenDestination) == "" {
		return config.Config{}, fmt.Errorf("no hay mutagenDestination en config; no se puede sincronizar")
	}
	if strings.TrimSpace(cfg.MutagenSessionName) == "" {
		return config.Config{}, fmt.Errorf("no hay mutagenSessionName en config; no se puede sincronizar")
	}
	return cfg, nil
}

func bootstrapConfigFromSlug() (config.Config, error) {
	slug := resolveWorkspaceSlug()
	if slug == "" {
		return config.Config{}, fmt.Errorf("slug de workspace vacío; usa --slug")
	}

	resolved, err := config.Resolve(apiURLFlag, tokenFlag)
	if err != nil {
		return config.Config{}, fmt.Errorf("no se pudo resolver config base (api/token): %w", err)
	}
	if strings.TrimSpace(resolved.Token) == "" {
		return config.Config{}, fmt.Errorf("falta token; ejecuta login o define EINAR_TOKEN")
	}

	client := api.NewClient(resolved.APIURL, resolved.Token, 30*time.Second)
	runtimeCfg, err := client.GetProjectBySlug(context.Background(), slug)
	if err != nil {
		return config.Config{}, fmt.Errorf("no se pudo obtener runtime config por slug %q: %w", slug, err)
	}

	localCfg := resolved
	localCfg.LastProjectID = strings.TrimSpace(runtimeCfg.ProjectID)
	localCfg.LastProjectSlug = strings.TrimSpace(runtimeCfg.Slug)
	localCfg.WorkspaceBranch = strings.TrimSpace(runtimeCfg.Workspace.Branch)
	localCfg.MutagenDestination = strings.TrimSpace(runtimeCfg.Sync.Destination)
	localCfg.MutagenSessionName = strings.TrimSpace(runtimeCfg.Sync.SessionName)
	localCfg.LastVMName = strings.TrimSpace(runtimeCfg.VM.Name)
	localCfg.LastVMHTTPSURL = strings.TrimSpace(runtimeCfg.VM.HTTPSURL)
	localCfg.LastVMSshDest = strings.TrimSpace(runtimeCfg.VM.SSHDestination)
	localCfg.ProjectDBName = strings.TrimSpace(runtimeCfg.Database.Name)
	localCfg.ProjectDBUser = strings.TrimSpace(runtimeCfg.Database.User)
	localCfg.ProjectDBHost = strings.TrimSpace(runtimeCfg.Database.Host)
	localCfg.ProjectDBPort = runtimeCfg.Database.Port
	if strings.TrimSpace(localCfg.MutagenDestination) == "" {
		localCfg.MutagenDestination = strings.TrimSpace(runtimeCfg.VM.SSHDestination)
	}

	if err := saveProjectConfig(localCfg); err != nil {
		return config.Config{}, fmt.Errorf("no se pudo guardar .einar/config.json rehidratado: %w", err)
	}
	fmt.Printf("✅ Config local rehidratada desde backend para slug '%s'\n", slug)
	return localCfg, nil
}

func resolveWorkspaceSlug() string {
	if s := strings.TrimSpace(workspaceSlug); s != "" {
		return s
	}
	if s := slugFromWorkspacesYAML(); s != "" {
		return s
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(filepath.Base(wd))
}

func slugFromWorkspacesYAML() string {
	b, err := os.ReadFile("workspaces.yaml")
	if err != nil {
		return ""
	}
	lines := strings.Split(string(b), "\n")
	inProject := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "project:" {
			inProject = true
			continue
		}
		if !inProject {
			continue
		}
		if !strings.HasPrefix(raw, " ") && !strings.HasPrefix(raw, "\t") {
			break
		}
		if strings.HasPrefix(line, "slug:") {
			slug := strings.TrimSpace(strings.TrimPrefix(line, "slug:"))
			slug = strings.Trim(slug, "\"'")
			return slug
		}
	}
	return ""
}

func runWorkspaceMutagenSync(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config de workspace inválida")
	}
	mutagenBin, err := resolveMutagenBinary()
	if err != nil {
		return err
	}
	if err := ensureMutagenYAMLForWorkspace(cfg); err != nil {
		return err
	}
	if err := startAndFlushMutagenProject(mutagenBin, cfg); err != nil {
		return err
	}
	if err := ensureMutagenSessionHealthy(cfg); err != nil {
		fmt.Printf("⚠️  Sesión mutagen no saludable, recreando proyecto y reintentando una vez...\n")
		if repairErr := recreateMutagenProject(mutagenBin, cfg); repairErr != nil {
			return fmt.Errorf("%w (además no se pudo recrear proyecto mutagen: %v)", err, repairErr)
		}
		if err := ensureMutagenSessionHealthy(cfg); err != nil {
			return err
		}
	}
	if err := triggerTopologyHeartbeatOnce(cfg); err != nil {
		fmt.Printf("⚠️  No se pudo refrescar topología inmediatamente: %v\n", err)
	}
	if err := startTopologyHeartbeatProcess(cfg); err != nil {
		fmt.Printf("⚠️  No se pudo iniciar heartbeat de topología: %v\n", err)
	}
	return nil
}

func startAndFlushMutagenProject(mutagenBin string, cfg *config.Config) error {
	start := exec.Command(mutagenBin, "project", "start")
	if out, err := start.CombinedOutput(); err != nil {
		msg := strings.ToLower(strings.TrimSpace(string(out)))
		if !strings.Contains(msg, "project already running") {
			if strings.TrimSpace(string(out)) != "" {
				return fmt.Errorf("mutagen project start falló: %s", strings.TrimSpace(string(out)))
			}
			return err
		}
	}

	sessionName := strings.TrimSpace(cfg.MutagenSessionName)
	if sessionName == "" {
		return fmt.Errorf("no hay mutagenSessionName en config; no se puede hacer flush de sync")
	}
	flush := exec.Command(mutagenBin, "sync", "flush", sessionName)
	if out, err := flush.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("mutagen sync flush falló: %s", msg)
		}
		return err
	}
	return nil
}

func recreateMutagenProject(mutagenBin string, cfg *config.Config) error {
	terminate := exec.Command(mutagenBin, "project", "terminate")
	if out, err := terminate.CombinedOutput(); err != nil {
		msg := strings.ToLower(strings.TrimSpace(string(out)))
		if !strings.Contains(msg, "no mutagen project found") && !strings.Contains(msg, "not found") {
			if strings.TrimSpace(string(out)) != "" {
				return fmt.Errorf("mutagen project terminate falló: %s", strings.TrimSpace(string(out)))
			}
			return err
		}
	}
	return startAndFlushMutagenProject(mutagenBin, cfg)
}

func ensureMutagenSessionHealthy(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config de workspace inválida")
	}
	mutagenBin, err := resolveMutagenBinary()
	if err != nil {
		return err
	}
	session := strings.TrimSpace(cfg.MutagenSessionName)
	if session == "" {
		return fmt.Errorf("mutagenSessionName vacío")
	}
	destination := strings.TrimSpace(cfg.MutagenDestination)
	if destination == "" {
		return fmt.Errorf("mutagenDestination vacío")
	}
	cmd := exec.Command(mutagenBin, "sync", "list", "--long", session)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text != "" {
			return fmt.Errorf("no se pudo validar sesión mutagen: %s", text)
		}
		return err
	}
	low := strings.ToLower(text)
	if !strings.Contains(text, destination) {
		return fmt.Errorf("sesión mutagen no apunta al destino esperado (%s)", destination)
	}
	if strings.Contains(low, "last error") && !strings.Contains(low, "last error: none") {
		return fmt.Errorf("sesión mutagen reporta errores: %s", text)
	}
	if strings.Contains(low, "problem") || strings.Contains(low, "disconnected") || strings.Contains(low, "unreachable") {
		return fmt.Errorf("sesión mutagen no está saludable: %s", text)
	}
	return nil
}

func ensureMutagenYAMLForWorkspace(cfg *config.Config) error {
	if _, err := os.Stat("mutagen.yml"); err == nil {
		return nil
	}
	if cfg == nil {
		return fmt.Errorf("config de workspace inválida para generar mutagen.yml")
	}
	destination := strings.TrimSpace(cfg.MutagenDestination)
	session := strings.TrimSpace(cfg.MutagenSessionName)
	if destination == "" {
		return fmt.Errorf("no hay mutagenDestination en config; no se puede generar mutagen.yml")
	}
	if session == "" {
		return fmt.Errorf("no hay mutagenSessionName en config; no se puede generar mutagen.yml")
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
    alpha: "."
    beta: "%s"
`, session, destination)
	if err := os.WriteFile("mutagen.yml", []byte(content), 0o644); err != nil {
		return fmt.Errorf("no se pudo generar mutagen.yml: %w", err)
	}
	fmt.Println("✅ mutagen.yml generado automáticamente para este workspace")
	return nil
}

func init() {
	takeCmd.Flags().StringVar(&workspaceSlug, "slug", "", "Slug del proyecto (si no existe .einar/config.json, por defecto usa nombre de carpeta)")
	rootCmd.AddCommand(takeCmd)
}
