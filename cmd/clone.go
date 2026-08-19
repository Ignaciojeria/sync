package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ignaciojeria/sync/internal/api"
	"github.com/Ignaciojeria/sync/internal/config"

	"github.com/spf13/cobra"
)

var (
	clonePathFlag string
	cloneNoSync   bool
)

var cloneCmd = &cobra.Command{
	Use:   "clone <slug>",
	Short: "Clona una VM existente en un workspace local mediante Mutagen",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := strings.TrimSpace(args[0])
		if slug == "" {
			return fmt.Errorf("slug requerido")
		}

		cfg, err := config.Resolve(apiURLFlag, tokenFlag)
		if err != nil {
			return err
		}
		if strings.TrimSpace(cfg.Token) == "" {
			return fmt.Errorf("falta token (usa 'login' o EINAR_TOKEN)")
		}

		client := api.NewClient(cfg.APIURL, cfg.Token, 30*time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		runtimeCfg, err := client.GetProjectBySlug(ctx, slug)
		if err != nil {
			if refreshed, rerr := shouldRefreshAndRetry(err, &cfg); rerr != nil {
				return rerr
			} else if refreshed {
				client = api.NewClient(cfg.APIURL, cfg.Token, 30*time.Second)
				runtimeCfg, err = client.GetProjectBySlug(ctx, slug)
			}
		}
		if err != nil {
			if msg := mapAPIError(err); msg != "" {
				return fmt.Errorf("%s", msg)
			}
			return fmt.Errorf("no se pudo obtener proyecto por slug %q: %w", slug, err)
		}

		targetDir, err := resolveCloneTargetDir(slug, clonePathFlag)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return fmt.Errorf("no se pudo crear carpeta local: %w", err)
		}
		if err := ensureCloneTargetIsReusable(targetDir, slug); err != nil {
			return err
		}

		prevWD, err := os.Getwd()
		if err != nil {
			return err
		}
		if err := os.Chdir(targetDir); err != nil {
			return fmt.Errorf("no se pudo entrar a la carpeta local: %w", err)
		}
		defer func() { _ = os.Chdir(prevWD) }()

		localCfg := hydrateProjectConfig(cfg, runtimeCfg)
		if err := ensureWorkspaceConfig(strings.TrimSpace(runtimeCfg.Slug)); err != nil {
			return fmt.Errorf("no se pudo preparar workspaces.yaml base: %w", err)
		}
		if err := ensureWorkspaceOwnership(&localCfg, forceWorkspaceTakeover); err != nil {
			return err
		}
		if err := saveProjectConfig(localCfg); err != nil {
			return fmt.Errorf("no se pudo guardar config local: %w", err)
		}

		fmt.Println("✅ Workspace local preparado")
		fmt.Printf("Slug: %s\n", strings.TrimSpace(runtimeCfg.Slug))
		if wd, werr := os.Getwd(); werr == nil {
			fmt.Printf("Local folder: %s\n", wd)
		}
		if strings.TrimSpace(localCfg.MutagenDestination) != "" {
			fmt.Printf("Mutagen destination: %s\n", strings.TrimSpace(localCfg.MutagenDestination))
		}

		if cloneNoSync {
			fmt.Println("ℹ️  Sync omitido (--no-sync)")
			return nil
		}

		if err := ensureMutagenOnWindows(); err != nil {
			return fmt.Errorf("workspace preparado, pero no se pudo verificar/instalar Mutagen: %w", err)
		}
		fmt.Println("✅ Mutagen disponible en esta máquina")

		if err := setupAndStartMutagen(&localCfg); err != nil {
			return fmt.Errorf("workspace preparado, pero no se pudo iniciar sync Mutagen: %w", err)
		}

		fmt.Println("✅ Workspace sincronizado")
		fmt.Println("Siguiente paso: entra al directorio y empieza a trabajar")
		return nil
	},
}

func hydrateProjectConfig(base config.Config, runtimeCfg *api.ProjectPublicConfig) config.Config {
	cfg := base
	if runtimeCfg == nil {
		return cfg
	}
	cfg.LastProjectID = strings.TrimSpace(runtimeCfg.ProjectID)
	cfg.LastProjectSlug = strings.TrimSpace(runtimeCfg.Slug)
	cfg.WorkspaceBranch = strings.TrimSpace(runtimeCfg.Workspace.Branch)
	cfg.MutagenDestination = strings.TrimSpace(runtimeCfg.Sync.Destination)
	cfg.MutagenSessionName = strings.TrimSpace(runtimeCfg.Sync.SessionName)
	cfg.LastVMName = strings.TrimSpace(runtimeCfg.VM.Name)
	cfg.LastVMHTTPSURL = strings.TrimSpace(runtimeCfg.VM.HTTPSURL)
	cfg.LastVMSshDest = strings.TrimSpace(runtimeCfg.VM.SSHDestination)
	cfg.ProjectDBName = strings.TrimSpace(runtimeCfg.Database.Name)
	cfg.ProjectDBUser = strings.TrimSpace(runtimeCfg.Database.User)
	cfg.ProjectDBHost = strings.TrimSpace(runtimeCfg.Database.Host)
	cfg.ProjectDBPort = runtimeCfg.Database.Port
	if strings.TrimSpace(cfg.MutagenDestination) == "" {
		cfg.MutagenDestination = strings.TrimSpace(runtimeCfg.VM.SSHDestination)
	}
	return cfg
}

func resolveCloneTargetDir(slug, rawPath string) (string, error) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		path = slug
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return absPath, nil
}

func ensureCloneTargetIsReusable(targetDir, slug string) error {
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(entries) == 0 {
		return nil
	}

	prevWD, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(targetDir); err != nil {
		return err
	}
	defer func() { _ = os.Chdir(prevWD) }()

	cfg, err := config.Load()
	if err == nil && strings.EqualFold(strings.TrimSpace(cfg.LastProjectSlug), strings.TrimSpace(slug)) {
		return nil
	}
	return fmt.Errorf("la carpeta destino ya existe y no está vacía: %s", targetDir)
}

func init() {
	cloneCmd.Flags().StringVar(&clonePathFlag, "path", "", "Carpeta local destino (default: ./<slug>)")
	cloneCmd.Flags().BoolVar(&cloneNoSync, "no-sync", false, "Prepara la carpeta local pero no inicia Mutagen")
	cloneCmd.Flags().BoolVar(&forceWorkspaceTakeover, "force-workspace-takeover", false, "Toma control del workspace global aunque otro owner lo tenga reservado")
	rootCmd.AddCommand(cloneCmd)
}
