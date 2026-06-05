package cmd

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/Ignaciojeria/sync/internal/config"
	"github.com/Ignaciojeria/sync/internal/tunnel"

	"github.com/spf13/cobra"
)

const defaultDBAPIURL = "https://postgresql.exe.xyz:8000"

var (
	dbProjectFlag  string
	dbAPIFlag      string
	dbHostFlag     string
	dbPortFlag     int
	dbDevSubFlag   string
	dbInsecureFlag bool
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Comandos para conexión a base de datos",
}

var dbConnectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Abre túnel local a PostgreSQL vía WebSocket",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Resolve(apiURLFlag, tokenFlag)
		if err != nil {
			return err
		}

		project := strings.TrimSpace(dbProjectFlag)
		if project == "" {
			project = firstNonEmpty(strings.TrimSpace(cfg.LastProjectSlug), strings.TrimSpace(cfg.LastProjectID))
		}
		if project == "" {
			return fmt.Errorf("falta proyecto (usa --project o ejecuta init en este workspace)")
		}

		token := strings.TrimSpace(cfg.Token)
		if token == "" && !dbInsecureFlag {
			return fmt.Errorf("falta token (usa 'einarc login' o EINAR_TOKEN)")
		}

		apiURL := resolveDBAPIURL(dbAPIFlag)
		listenAddr := net.JoinHostPort(strings.TrimSpace(dbHostFlag), strconv.Itoa(dbPortFlag))

		wsURL, err := tunnel.TunnelURL(apiURL, project)
		if err != nil {
			return err
		}

		fmt.Printf("✅ DB tunnel listo\n")
		fmt.Printf("Project: %s\n", project)
		fmt.Printf("Listen:  %s\n", listenAddr)
		fmt.Printf("Remote:  %s\n", wsURL)

		if localURL, err := localDatabaseURL(cfg.ProjectDatabaseURL, strings.TrimSpace(dbHostFlag), dbPortFlag); err == nil && localURL != "" {
			fmt.Printf("Database URL: %s\n", localURL)
		}
		fmt.Println("Mantén este proceso corriendo mientras usas psql/DBeaver.")

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		return tunnel.Serve(ctx, listenAddr, tunnel.Options{
			APIBaseURL:   apiURL,
			Project:      project,
			Token:        token,
			DevSub:       strings.TrimSpace(dbDevSubFlag),
			InsecureAuth: dbInsecureFlag,
		}, cmd.Printf)
	},
}

func init() {
	dbConnectCmd.Flags().StringVar(&dbProjectFlag, "project", "", "ID o nombre de proyecto (default: config local)")
	dbConnectCmd.Flags().StringVar(&dbAPIFlag, "api", envOrDefault("EINAR_DB_API_URL", defaultDBAPIURL), "Base URL del Postgres API")
	dbConnectCmd.Flags().StringVar(&dbHostFlag, "host", "127.0.0.1", "Host local para exponer el túnel")
	dbConnectCmd.Flags().IntVar(&dbPortFlag, "port", 15432, "Puerto local para exponer el túnel")
	dbConnectCmd.Flags().StringVar(&dbDevSubFlag, "dev-sub", envOrDefault("EINAR_DEV_SUB", "dev-user"), "X-Dev-Sub para testing local")
	dbConnectCmd.Flags().BoolVar(&dbInsecureFlag, "insecure-unauthenticated-local-test", false, "omite Authorization para testing local")

	dbCmd.AddCommand(dbConnectCmd)
	rootCmd.AddCommand(dbCmd)
}

func resolveDBAPIURL(flagValue string) string {
	v := strings.TrimSpace(flagValue)
	if v != "" {
		return strings.TrimRight(v, "/")
	}
	envValue := strings.TrimSpace(os.Getenv("EINAR_DB_API_URL"))
	if envValue != "" {
		return strings.TrimRight(envValue, "/")
	}
	return defaultDBAPIURL
}

func localDatabaseURL(rawURL, host string, port int) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	u.Host = net.JoinHostPort(host, strconv.Itoa(port))
	query := u.Query()
	if _, ok := query["sslmode"]; !ok {
		query.Set("sslmode", "disable")
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}
