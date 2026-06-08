package cmd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Ignaciojeria/sync/internal/config"
	"github.com/Ignaciojeria/sync/internal/machineauth"
	"github.com/Ignaciojeria/sync/internal/tunnel"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/spf13/cobra"
)

const (
	defaultDBAPIURL          = "https://postgresql.exe.xyz:8000"
	defaultScaffoldMigrations = "internal/shared/infrastructure/postgresql/migrations"
)

var (
	dbProjectFlag        string
	dbAPIFlag            string
	dbHostFlag           string
	dbPortFlag           int
	dbDevSubFlag         string
	dbInsecureFlag       bool
	dbMigrateDirFlag     string
	dbMigrateDatabaseURL string
	dbMigrateNoSSHFlag   bool
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Comandos para conexión a base de datos",
}

var dbConnectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Abre túnel local a PostgreSQL vía WebSocket",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := resolveDBCommandConfig(tokenFlag)
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
		var tokenProvider func(context.Context) (string, error)
		if !dbInsecureFlag {
			machineSource, err := machineTokenSourceFromConfig(cfg)
			if err != nil {
				return err
			}
			if machineSource != nil {
				tokenProvider = machineSource.Token
				token = ""
			} else if token == "" {
				return fmt.Errorf("falta token (usa 'einarc login' o EINAR_TOKEN) y no hay machineAuth configurado para este proyecto")
			}
		}

		apiURL := resolveDBAPIURL(dbAPIFlag)
		listenAddr := net.JoinHostPort(strings.TrimSpace(dbHostFlag), strconv.Itoa(dbPortFlag))

		wsURL, err := tunnel.TunnelURL(apiURL, project)
		if err != nil {
			return err
		}

		fmt.Printf("✅ DB tunnel listo\n")
		if tokenProvider != nil {
			fmt.Println("Auth: machineAuth del proyecto actual")
		} else if !dbInsecureFlag {
			fmt.Println("Auth: bearer token del usuario")
		}
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
			APIBaseURL:    apiURL,
			Project:       project,
			Token:         token,
			TokenProvider: tokenProvider,
			DevSub:        strings.TrimSpace(dbDevSubFlag),
			InsecureAuth:  dbInsecureFlag,
		}, cmd.Printf)
	},
}

var dbMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Ejecuta las migraciones SQL del proyecto",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := resolveDBCommandConfig(tokenFlag)
		if err != nil {
			return err
		}
		migrationsDir := strings.TrimSpace(dbMigrateDirFlag)
		if migrationsDir == "" {
			migrationsDir = defaultScaffoldMigrations
		}
		if err := runProjectMigrationsWithCurrentConfig(cfg, migrationsDir, strings.TrimSpace(dbMigrateDatabaseURL), !dbMigrateNoSSHFlag); err != nil {
			return err
		}
		fmt.Println("✅ Migraciones aplicadas correctamente")
		return nil
	},
}

func init() {
	dbConnectCmd.Flags().StringVar(&dbProjectFlag, "project", "", "ID o nombre de proyecto (default: config local)")
	dbConnectCmd.Flags().StringVar(&dbAPIFlag, "api", envOrDefault("EINAR_DB_API_URL", defaultDBAPIURL), "Base URL del Postgres API")
	dbConnectCmd.Flags().StringVar(&dbHostFlag, "host", "127.0.0.1", "Host local para exponer el túnel")
	dbConnectCmd.Flags().IntVar(&dbPortFlag, "port", 15432, "Puerto local para exponer el túnel")
	dbConnectCmd.Flags().StringVar(&dbDevSubFlag, "dev-sub", envOrDefault("EINAR_DEV_SUB", "dev-user"), "X-Dev-Sub para testing local")
	dbConnectCmd.Flags().BoolVar(&dbInsecureFlag, "insecure-unauthenticated-local-test", false, "omite Authorization para testing local")

	dbMigrateCmd.Flags().StringVar(&dbMigrateDirFlag, "dir", defaultScaffoldMigrations, "Carpeta de migraciones SQL del proyecto")
	dbMigrateCmd.Flags().StringVar(&dbMigrateDatabaseURL, "database-url", "", "DATABASE_URL explícita (default: config del proyecto)")
	dbMigrateCmd.Flags().BoolVar(&dbMigrateNoSSHFlag, "no-ssh-forward", false, "No abre un port-forward SSH temporal hacia la VM")

	dbCmd.AddCommand(dbConnectCmd)
	dbCmd.AddCommand(dbMigrateCmd)
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

func resolveDBCommandConfig(tokenFlag string) (config.Config, error) {
	projectCfg, err := config.Load()
	if err != nil {
		return config.Config{}, err
	}
	globalCfg, _ := config.LoadGlobal()

	cfg := projectCfg
	cfg.Token = strings.TrimSpace(globalCfg.Token)
	cfg.RefreshToken = strings.TrimSpace(globalCfg.RefreshToken)
	cfg.APIURL = strings.TrimSpace(globalCfg.APIURL)

	if envToken := strings.TrimSpace(os.Getenv("EINAR_TOKEN")); envToken != "" {
		cfg.Token = envToken
	}
	if flagToken := strings.TrimSpace(tokenFlag); flagToken != "" {
		cfg.Token = flagToken
	}
	return cfg, nil
}

func machineTokenSourceFromConfig(cfg config.Config) (*machineauth.TokenSource, error) {
	if strings.TrimSpace(cfg.MachineAuthTokenEndpoint) == "" && strings.TrimSpace(cfg.MachineAuthClientID) == "" && strings.TrimSpace(cfg.MachineAuthClientSecret) == "" {
		return nil, nil
	}
	return machineauth.NewTokenSource(machineauth.Config{
		GrantType:     strings.TrimSpace(cfg.MachineAuthGrantType),
		TokenEndpoint: strings.TrimSpace(cfg.MachineAuthTokenEndpoint),
		ClientID:      strings.TrimSpace(cfg.MachineAuthClientID),
		ClientSecret:  strings.TrimSpace(cfg.MachineAuthClientSecret),
		Audience:      strings.TrimSpace(cfg.MachineAuthAudience),
		Scopes:        strings.Fields(strings.TrimSpace(cfg.MachineAuthScopes)),
	})
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}

func runProjectMigrationsWithCurrentConfig(cfg config.Config, migrationsDir, explicitDatabaseURL string, allowSSHForward bool) error {
	migrationsDir = strings.TrimSpace(migrationsDir)
	if migrationsDir == "" {
		return fmt.Errorf("carpeta de migraciones vacía")
	}
	if stat, err := os.Stat(migrationsDir); err != nil || !stat.IsDir() {
		if err != nil {
			return fmt.Errorf("carpeta de migraciones no encontrada en %s: %w", migrationsDir, err)
		}
		return fmt.Errorf("ruta de migraciones no es carpeta: %s", migrationsDir)
	}

	databaseURL := strings.TrimSpace(explicitDatabaseURL)
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(cfg.ProjectDatabaseURL)
	}
	if databaseURL == "" {
		return fmt.Errorf("falta DATABASE_URL del proyecto")
	}

	if allowSSHForward {
		forwardedURL, cleanup, err := forwardedDatabaseURLViaVM(cfg, databaseURL)
		if err == nil {
			defer cleanup()
			databaseURL = forwardedURL
		} else {
			fmt.Printf("ℹ️  No se pudo abrir port-forward SSH temporal; se usará DATABASE_URL directa (%v)\n", err)
		}
	}

	if err := applyMigrations(databaseURL, migrationsDir); err != nil {
		return err
	}
	return nil
}

func forwardedDatabaseURLViaVM(cfg config.Config, rawDatabaseURL string) (string, func(), error) {
	destination := normalizeMutagenDestinationForProject(strings.TrimSpace(cfg.MutagenDestination))
	if destination == "" {
		destination = strings.TrimSpace(cfg.LastVMSshDest)
	}
	target, _, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok || strings.TrimSpace(target) == "" {
		return "", nil, fmt.Errorf("no se pudo resolver target SSH de la VM")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	localPort := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	sshCmd := newSSHCommand(
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"-L", fmt.Sprintf("127.0.0.1:%d:127.0.0.1:%d", localPort, defaultRemoteDBListenPort),
		target,
		"-N",
	)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	if err := sshCmd.Start(); err != nil {
		return "", nil, fmt.Errorf("no se pudo iniciar port-forward SSH: %w", err)
	}

	cleanup := func() {
		if sshCmd.Process != nil {
			_ = sshCmd.Process.Kill()
			_, _ = sshCmd.Process.Wait()
		}
	}

	if err := waitForLocalTCP(fmt.Sprintf("127.0.0.1:%d", localPort), 10*time.Second); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("port-forward SSH no quedó listo: %w", err)
	}

	forwardedURL, err := localDatabaseURL(rawDatabaseURL, "127.0.0.1", localPort)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return forwardedURL, cleanup, nil
}

func waitForLocalTCP(address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 700*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("timeout esperando %s", address)
}

func applyMigrations(databaseURL, migrationsDir string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("no se pudo abrir conexión a postgres: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("no se pudo conectar a postgres: %w", err)
	}

	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	if err != nil {
		return fmt.Errorf("no se pudo crear driver de migraciones: %w", err)
	}

	absDir, err := filepath.Abs(migrationsDir)
	if err != nil {
		return err
	}
	absDir = filepath.ToSlash(absDir)
	if !strings.HasPrefix(absDir, "/") {
		absDir = "/" + absDir
	}
	sourceURL := "file://" + absDir

	m, err := migrate.NewWithDatabaseInstance(sourceURL, "postgres", driver)
	if err != nil {
		return fmt.Errorf("no se pudo crear migrator: %w", err)
	}
	defer func() {
		_, _ = m.Close()
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("falló migrate up: %w", err)
	}
	return nil
}
