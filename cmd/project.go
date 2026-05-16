package cmd

import (
	"context"
	"encoding/json"
	"fmt"
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
	skipMutagenCheck     bool
	initMutagenDestination string
	initMutagenName        string
	skipMutagenStart      bool
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
			if msg := mapAPIError(err); msg != "" {
				return fmt.Errorf("%s", msg)
			}
			return err
		}
		if strings.TrimSpace(resp.ProjectID) == "" || strings.TrimSpace(resp.Slug) == "" {
			return fmt.Errorf("respuesta inválida del backend: projectId/slug vacíos (status=%q)", strings.TrimSpace(resp.Status))
		}

		cfg.LastProjectID = strings.TrimSpace(resp.ProjectID)
		cfg.LastProjectSlug = strings.TrimSpace(resp.Slug)
		if strings.TrimSpace(resp.MutagenDestination) != "" {
			cfg.MutagenDestination = strings.TrimSpace(resp.MutagenDestination)
		} else if strings.TrimSpace(resp.VMSshDest) != "" {
			cfg.MutagenDestination = strings.TrimSpace(resp.VMSshDest)
		}
		if strings.TrimSpace(resp.MutagenSessionName) != "" {
			cfg.MutagenSessionName = strings.TrimSpace(resp.MutagenSessionName)
		}
		if strings.TrimSpace(resp.VMName) != "" {
			cfg.LastVMName = strings.TrimSpace(resp.VMName)
		}
		if strings.TrimSpace(resp.VMHTTPSURL) != "" {
			cfg.LastVMHTTPSURL = strings.TrimSpace(resp.VMHTTPSURL)
		}
		if strings.TrimSpace(resp.VMSshDest) != "" {
			cfg.LastVMSshDest = strings.TrimSpace(resp.VMSshDest)
		}
		if strings.TrimSpace(resp.ProjectAPIToken) != "" {
			cfg.ProjectAPIToken = strings.TrimSpace(resp.ProjectAPIToken)
		}
		if strings.TrimSpace(resp.VMSshPrivateKey) != "" {
			if err := writeSSHPrivateKey(resp.VMSshPrivateKey); err != nil {
				return fmt.Errorf("no se pudo guardar clave SSH privada: %w", err)
			}
		}
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("no se pudo guardar config local: %w", err)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(resp)
		}

		fmt.Println("✅ Proyecto creado")
		fmt.Printf("ID: %s\n", resp.ProjectID)
		fmt.Printf("Name: %s\n", resp.Name)
		fmt.Printf("Slug: %s\n", resp.Slug)
		fmt.Printf("Subdomain: %s\n", resp.Subdomain)
		fmt.Printf("Status: %s\n", resp.Status)
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

		if !skipMutagenCheck {
			if err := ensureMutagenOnWindows(); err != nil {
				fmt.Printf("⚠️  Proyecto creado, pero no se pudo verificar/instalar Mutagen: %v\n", err)
				fmt.Println("   Puedes instalarlo manualmente y luego correr: einarc mutagen ...")
			} else {
				fmt.Println("✅ Mutagen disponible en esta máquina")
				if err := setupAndStartMutagen(&cfg); err != nil {
					fmt.Printf("⚠️  Mutagen disponible, pero no se pudo auto-configurar/start: %v\n", err)
					fmt.Println("   Puedes correr manualmente: einarc mutagen --destination <destino> && mutagen project start")
				}
			}
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

func ensureMutagenOnWindows() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	if _, err := exec.LookPath("mutagen"); err == nil {
		return nil
	}

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

	return fmt.Errorf("mutagen no está en PATH y no se pudo instalar con winget/choco")
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

func setupAndStartMutagen(cfg *config.Config) error {
	if _, err := exec.LookPath("mutagen"); err != nil {
		return fmt.Errorf("mutagen no está en PATH")
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
		sessionName = "dev-sync"
	}

	if _, err := os.Stat("mutagen.yml"); os.IsNotExist(err) {
		if destination == "" {
			return fmt.Errorf("falta destino mutagen (usa --mutagen-destination o configura PROJECTS_SYNC_SSH_* en backend)")
		}
		source, err := os.Getwd()
		if err != nil {
			return err
		}
		absSource, err := filepath.Abs(source)
		if err != nil {
			return err
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
`, sessionName, absSource, destination)
		if err := os.WriteFile("mutagen.yml", []byte(content), 0o644); err != nil {
			return err
		}
		fmt.Println("✅ mutagen.yml generado automáticamente")
	}

	if skipMutagenStart {
		fmt.Println("ℹ️  Se omitió 'mutagen project start' por --skip-mutagen-start")
		return nil
	}

	cmd := exec.Command("mutagen", "project", "start")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Println("✅ Mutagen sync iniciado (mutagen project start)")
	return nil
}

func init() {
	initCmd.Flags().BoolVar(&skipMutagenCheck, "skip-mutagen-check", false, "No verifica/instala Mutagen automáticamente")
	initCmd.Flags().StringVar(&initMutagenDestination, "mutagen-destination", "", "Destino remoto Mutagen (ej: docker://mi-contenedor/var/www)")
	initCmd.Flags().StringVar(&initMutagenName, "mutagen-name", "dev-sync", "Nombre de sesión en mutagen.yml")
	initCmd.Flags().BoolVar(&skipMutagenStart, "skip-mutagen-start", false, "No ejecuta 'mutagen project start'")
	rootCmd.AddCommand(initCmd)
}
