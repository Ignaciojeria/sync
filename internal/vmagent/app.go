package vmagent

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ignaciojeria/sync/internal/machineauth"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type status struct {
	Healthy      bool   `json:"healthy"`
	Message      string `json:"message,omitempty"`
	UpdatedAt    string `json:"updatedAt"`
	DatabaseURL  string `json:"databaseUrl,omitempty"`
	TokenChecked bool   `json:"tokenChecked"`
}

func Execute() {
	if err := Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func Run() error {
	if len(os.Args) < 2 {
		printUsage()
		return nil
	}

	switch os.Args[1] {
	case "db-connect":
		return runDBConnect(os.Args[2:])
	case "db-health":
		return runDBHealth(os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		return fmt.Errorf("comando vm-agent no soportado: %s", os.Args[1])
	}
}

func printUsage() {
	fmt.Println("vm-agent commands:")
	fmt.Println("  db-connect   valida auth machine + conectividad DB en loop y escribe status")
	fmt.Println("  db-health    valida auth machine + conectividad DB una vez")
}

func runDBConnect(args []string) error {
	fs := flag.NewFlagSet("db-connect", flag.ContinueOnError)
	statusPath := fs.String("status-file", "~/.einar/db-connect-status.json", "Ruta del archivo de status")
	interval := fs.Duration("interval", 30*time.Second, "Intervalo entre validaciones")
	fs.SetOutput(os.Stdout)
	if err := fs.Parse(args); err != nil {
		return err
	}

	resolvedStatusPath, err := expandPath(*statusPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolvedStatusPath), 0o700); err != nil {
		return err
	}

	for {
		st := checkConnectivity(context.Background())
		if err := writeStatus(resolvedStatusPath, st); err != nil {
			fmt.Printf("warning: no se pudo escribir status: %v\n", err)
		}
		if st.Healthy {
			fmt.Println("db-connect: ok")
		} else {
			fmt.Printf("db-connect: %s\n", st.Message)
		}
		time.Sleep(*interval)
	}
}

func runDBHealth(args []string) error {
	fs := flag.NewFlagSet("db-health", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	if err := fs.Parse(args); err != nil {
		return err
	}
	st := checkConnectivity(context.Background())
	b, _ := json.MarshalIndent(st, "", "  ")
	fmt.Println(string(b))
	if !st.Healthy {
		return fmt.Errorf("%s", st.Message)
	}
	return nil
}

func checkConnectivity(ctx context.Context) status {
	st := status{UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	st.DatabaseURL = maskDatabaseURL(databaseURL)
	if databaseURL == "" {
		st.Message = "DATABASE_URL vacío"
		return st
	}

	machineCfg := machineauth.Config{
		GrantType:     firstNonEmpty(os.Getenv("MACHINE_AUTH_GRANT_TYPE"), "client_credentials"),
		TokenEndpoint: firstNonEmpty(os.Getenv("MACHINE_AUTH_TOKEN_ENDPOINT"), os.Getenv("OIDC_TOKEN_ENDPOINT")),
		ClientID:      firstNonEmpty(os.Getenv("MACHINE_AUTH_CLIENT_ID"), os.Getenv("OIDC_CLIENT_ID")),
		ClientSecret:  firstNonEmpty(os.Getenv("MACHINE_AUTH_CLIENT_SECRET"), os.Getenv("OIDC_CLIENT_SECRET")),
		Audience:      os.Getenv("MACHINE_AUTH_AUDIENCE"),
		Scopes:        strings.Fields(os.Getenv("MACHINE_AUTH_SCOPES")),
	}
	if machineCfg.TokenEndpoint != "" && machineCfg.ClientID != "" && machineCfg.ClientSecret != "" {
		ts, err := machineauth.NewTokenSource(machineCfg)
		if err != nil {
			st.Message = fmt.Sprintf("machine auth config inválida: %v", err)
			return st
		}
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if _, err := ts.Token(ctx); err != nil {
			st.Message = fmt.Sprintf("no se pudo obtener machine token: %v", err)
			return st
		}
		st.TokenChecked = true
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		st.Message = fmt.Sprintf("sql open failed: %v", err)
		return st
	}
	defer db.Close()
	db.SetConnMaxLifetime(30 * time.Second)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		st.Message = fmt.Sprintf("db ping failed: %v", err)
		return st
	}
	var one int
	if err := db.QueryRowContext(pingCtx, "select 1").Scan(&one); err != nil {
		st.Message = fmt.Sprintf("db query failed: %v", err)
		return st
	}
	st.Healthy = true
	st.Message = "ok"
	return st
}

func writeStatus(path string, st status) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func expandPath(path string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", fmt.Errorf("path vacío")
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			return home, nil
		}
		return filepath.Join(home, strings.TrimPrefix(p, "~/")), nil
	}
	return p, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func maskDatabaseURL(raw string) string {
	parts := strings.Split(raw, "@")
	if len(parts) != 2 {
		return raw
	}
	left := parts[0]
	idx := strings.LastIndex(left, ":")
	if idx < 0 {
		return raw
	}
	return left[:idx+1] + "***@" + parts[1]
}
