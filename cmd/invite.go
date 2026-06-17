package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Ignaciojeria/sync/internal/api"
	"github.com/Ignaciojeria/sync/internal/config"
	"github.com/spf13/cobra"
)

var inviteSlug string

var inviteCmd = &cobra.Command{
	Use:   "invite <email>",
	Short: "Invita un miembro al proyecto actual",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		email := strings.TrimSpace(args[0])
		if email == "" {
			return fmt.Errorf("email requerido")
		}

		cfg, err := config.Resolve(apiURLFlag, tokenFlag)
		if err != nil {
			return err
		}
		if strings.TrimSpace(cfg.Token) == "" {
			return fmt.Errorf("falta token (usa 'login' o EINAR_TOKEN)")
		}

		slug := strings.TrimSpace(inviteSlug)
		if slug == "" {
			slug = strings.TrimSpace(cfg.LastProjectSlug)
		}
		if slug == "" {
			slug = strings.TrimSpace(resolveWorkspaceSlug())
		}
		if slug == "" {
			return fmt.Errorf("no se pudo resolver el slug del proyecto actual; usa --slug")
		}

		client := api.NewClient(cfg.APIURL, cfg.Token, 10*time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if debugHTTP {
			fmt.Println("[debug] HTTP request")
			fmt.Printf("[debug] POST %s/api/projects/%s/invite\n", cfg.APIURL, slug)
			fmt.Printf("[debug] Authorization: Bearer %s\n", config.MaskToken(cfg.Token))
			fmt.Printf("[debug] Body: {\"email\":%q}\n", email)
		}

		resp, err := client.InviteProjectMember(ctx, slug, email)
		if err != nil {
			if refreshed, rerr := shouldRefreshAndRetry(err, &cfg); rerr != nil {
				return rerr
			} else if refreshed {
				client = api.NewClient(cfg.APIURL, cfg.Token, 10*time.Second)
				resp, err = client.InviteProjectMember(ctx, slug, email)
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
			if msg := mapInviteAPIError(err); msg != "" {
				return fmt.Errorf("%s", msg)
			}
			return err
		}

		if jsonOutput {
			b, merr := json.MarshalIndent(resp, "", "  ")
			if merr != nil {
				return merr
			}
			fmt.Println(string(b))
			return nil
		}

		fmt.Printf("✅ Invitación enviada a %s en proyecto '%s'\n", resp.Email, firstNonEmptyTrimmed(resp.Slug, slug))
		if strings.TrimSpace(resp.Role) != "" {
			fmt.Printf("   role: %s\n", resp.Role)
		}
		fmt.Printf("   granted: %t\n", resp.Granted)
		return nil
	},
}

func mapInviteAPIError(err error) string {
	var ae api.APIError
	if !api.AsAPIError(err, &ae) {
		return ""
	}

	switch ae.StatusCode {
	case 401:
		return "Token inválido/revocado/expirado. Ejecuta login nuevamente."
	case 403:
		lowerCode := strings.ToLower(strings.TrimSpace(ae.Code))
		lowerMsg := strings.ToLower(strings.TrimSpace(ae.Message))
		if strings.Contains(lowerCode, "scope") || strings.Contains(lowerMsg, "scope") {
			return "Tu token no incluye scope projects:create."
		}
		return "Solo el owner del proyecto puede invitar miembros."
	case 404:
		return "No se encontró el proyecto para ese slug."
	case 409:
		return "Ese email ya tiene acceso al proyecto."
	default:
		return fmt.Sprintf("Error API (%d): %s", ae.StatusCode, ae.Message)
	}
}

func init() {
	inviteCmd.Flags().StringVar(&inviteSlug, "slug", "", "Slug del proyecto (por defecto: .einar/config.json, workspaces.yaml o carpeta actual)")
	rootCmd.AddCommand(inviteCmd)
}
