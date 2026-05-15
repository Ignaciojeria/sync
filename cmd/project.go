package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"einarc/internal/api"
	"einarc/internal/config"

	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Operaciones de proyectos",
}

var projectCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Crea un proyecto",
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
			return fmt.Errorf("falta token (usa 'login --token <PAT>' o EINAR_TOKEN)")
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

func init() {
	projectCmd.AddCommand(projectCreateCmd)
	rootCmd.AddCommand(projectCmd)
}
