package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ignaciojeria/sync/internal/config"
	"github.com/spf13/cobra"
)

const defaultRemoteCLIBinaryName = "sync"
const defaultRemoteDBServiceName = "einar-db-connect.service"
const defaultRemoteDBListenHost = "127.0.0.1"
const defaultRemoteDBListenPort = 15432

var vmAgentCmd = &cobra.Command{Use: "vm-agent", Short: "Bootstrap remoto del CLI en la VM"}

var vmAgentBootstrapDBCmd = &cobra.Command{
	Use:   "bootstrap-db",
	Short: "Compila el CLI, lo sube a la VM y arranca sync db connect como servicio",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		return ensureDBBootstrapOnProvisionedVM(&cfg)
	},
}

var vmAgentStatusDBCmd = &cobra.Command{
	Use:   "db-status",
	Short: "Muestra estado remoto del túnel DB",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, target, remotePath, err := loadVMTargetConfig()
		if err != nil {
			return err
		}
		_ = cfg
		remoteBinaryPath := strings.TrimRight(remotePath, "/") + "/" + defaultRemoteCLIBinaryName
		script := fmt.Sprintf(`echo "service=$(sudo systemctl is-active %s 2>/dev/null || true)"; echo "binary=%s"; echo "workspace=%s"; ss -ltnp | grep ':%d ' || true`, defaultRemoteDBServiceName, shellQuote(remoteBinaryPath), shellQuote(remotePath), defaultRemoteDBListenPort)
		out, err := runSSHScriptWithTimeout(target, script, 20*time.Second)
		if strings.TrimSpace(out) != "" {
			fmt.Println(strings.TrimSpace(out))
		}
		return err
	},
}

func init() {
	vmAgentCmd.AddCommand(vmAgentBootstrapDBCmd)
	vmAgentCmd.AddCommand(vmAgentStatusDBCmd)
	rootCmd.AddCommand(vmAgentCmd)
}

func ensureDBBootstrapOnProvisionedVM(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config inválida para bootstrap DB remoto")
	}
	_, target, remotePath, err := resolveVMRemote(*cfg)
	if err != nil {
		return err
	}
	destination := normalizeMutagenDestinationForProject(strings.TrimSpace(cfg.MutagenDestination))
	if err := ensureLocalSSHKeySetup(); err != nil {
		fmt.Printf("⚠️  No se pudo preparar clave SSH local automáticamente: %v\n", err)
	}
	if err := ensureExeDevSSHOnboarding(destination); err != nil {
		return err
	}
	if err := ensureSSHTrustInteractive(destination); err != nil {
		return err
	}
	if err := preflightSSHConnection(destination); err != nil {
		return err
	}
	if err := buildAndUploadRemoteCLI(target, remotePath); err != nil {
		return err
	}
	if err := uploadRemoteProjectConfig(target, remotePath); err != nil {
		return err
	}
	if err := installRemoteDBService(target, remotePath, cfg); err != nil {
		return err
	}
	if err := verifyRemoteDBService(target, remotePath, cfg); err != nil {
		return err
	}
	fmt.Println("✅ Bootstrap remoto DB completado: CLI subido y sync db connect corriendo en la VM")
	return nil
}

func buildAndUploadRemoteCLI(target, remotePath string) error {
	output := filepath.Join(".einar", "bin", "sync-linux-amd64")
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	repoRoot, err := resolveLocalCLISourceRoot()
	if err != nil {
		return err
	}
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-o", absOutput, ".")
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build CLI remoto falló: %w", err)
	}
	fmt.Printf("✅ CLI Linux compilado en %s\n", absOutput)
	remoteBinaryPath := strings.TrimRight(remotePath, "/") + "/" + defaultRemoteCLIBinaryName
	if _, err := runSSHScript(target, fmt.Sprintf("mkdir -p %s", shellQuote(remotePath))); err != nil {
		return fmt.Errorf("no se pudo preparar raíz remota del proyecto: %w", err)
	}
	scp := exec.Command("scp", absOutput, target+":"+remoteBinaryPath)
	scp.Stdout = os.Stdout
	scp.Stderr = os.Stderr
	if err := scp.Run(); err != nil {
		return fmt.Errorf("scp CLI remoto falló: %w", err)
	}
	if _, err := runSSHScript(target, fmt.Sprintf("chmod +x %s", shellQuote(remoteBinaryPath))); err != nil {
		return fmt.Errorf("no se pudo marcar CLI remoto como ejecutable: %w", err)
	}
	fmt.Printf("✅ CLI subido a %s:%s\n", target, remoteBinaryPath)
	return nil
}

func resolveLocalCLISourceRoot() (string, error) {
	candidates := []string{}
	if exePath, err := os.Executable(); err == nil && strings.TrimSpace(exePath) != "" {
		candidates = append(candidates, filepath.Dir(exePath))
	}
	if wd, err := os.Getwd(); err == nil && strings.TrimSpace(wd) != "" {
		candidates = append(candidates, wd)
	}
	for _, candidate := range candidates {
		root := strings.TrimSpace(candidate)
		if root == "" {
			continue
		}
		if stat, err := os.Stat(filepath.Join(root, "cmd")); err == nil && stat.IsDir() {
			if _, err := os.Stat(filepath.Join(root, "main.go")); err == nil {
				return root, nil
			}
		}
		if parent := filepath.Dir(root); parent != root {
			if stat, err := os.Stat(filepath.Join(parent, "cmd")); err == nil && stat.IsDir() {
				if _, err := os.Stat(filepath.Join(parent, "main.go")); err == nil {
					return parent, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no se pudo ubicar el source root del CLI para compilar el binario remoto")
}

func uploadRemoteProjectConfig(target, remotePath string) error {
	localConfigPath, err := config.ConfigPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(localConfigPath); err != nil {
		return fmt.Errorf("config local del proyecto no encontrada en %s: %w", localConfigPath, err)
	}
	remoteConfigDir := strings.TrimRight(remotePath, "/") + "/.einar"
	remoteConfigPath := remoteConfigDir + "/config.json"
	if _, err := runSSHScript(target, fmt.Sprintf("mkdir -p %s", shellQuote(remoteConfigDir))); err != nil {
		return fmt.Errorf("no se pudo preparar .einar remoto: %w", err)
	}
	scp := exec.Command("scp", localConfigPath, target+":"+remoteConfigPath)
	scp.Stdout = os.Stdout
	scp.Stderr = os.Stderr
	if err := scp.Run(); err != nil {
		return fmt.Errorf("scp config.json remoto falló: %w", err)
	}
	if _, err := runSSHScript(target, fmt.Sprintf("chmod 600 %s", shellQuote(remoteConfigPath))); err != nil {
		return fmt.Errorf("no se pudo proteger config.json remoto: %w", err)
	}
	fmt.Printf("✅ Config del proyecto subida a %s\n", remoteConfigPath)
	return nil
}

func installRemoteDBService(target, remotePath string, cfg *config.Config) error {
	projectRef := firstNonEmptyTrimmed(strings.TrimSpace(cfg.LastProjectSlug), strings.TrimSpace(cfg.LastProjectID))
	if projectRef == "" {
		return fmt.Errorf("falta lastProjectSlug/lastProjectID en config local")
	}
	remoteRoot := strings.TrimRight(strings.TrimSpace(remotePath), "/")
	remoteBinaryPath := remoteRoot + "/" + defaultRemoteCLIBinaryName
	service := fmt.Sprintf(`[Unit]
Description=Einar DB tunnel via CLI
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s db connect --project %s --host %s --port %d
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, remoteRoot, remoteBinaryPath, projectRef, defaultRemoteDBListenHost, defaultRemoteDBListenPort)
	script := fmt.Sprintf("cat > /tmp/%s <<'EOF'\n%s\nEOF\nsudo mv /tmp/%s /etc/systemd/system/%s\nsudo systemctl daemon-reload\nsudo systemctl enable %s\nsudo systemctl restart %s\n", defaultRemoteDBServiceName, service, defaultRemoteDBServiceName, defaultRemoteDBServiceName, defaultRemoteDBServiceName, defaultRemoteDBServiceName)
	out, err := runSSHScriptWithTimeout(target, script, 60*time.Second)
	if strings.TrimSpace(out) != "" {
		fmt.Println(strings.TrimSpace(out))
	}
	if err != nil {
		statusOut, _ := runSSHScriptWithTimeout(target, fmt.Sprintf("sudo systemctl cat %s || true\nprintf '\n---\n'\nsudo systemctl status %s --no-pager -l || true\nprintf '\n---\n'\nsudo journalctl -u %s -n 50 --no-pager || true", defaultRemoteDBServiceName, defaultRemoteDBServiceName, defaultRemoteDBServiceName), 30*time.Second)
		if strings.TrimSpace(statusOut) != "" {
			return fmt.Errorf("no se pudo instalar servicio remoto DB: %w\n%s", err, strings.TrimSpace(statusOut))
		}
		return fmt.Errorf("no se pudo instalar servicio remoto DB: %w", err)
	}
	fmt.Printf("✅ Servicio %s instalado\n", defaultRemoteDBServiceName)
	return nil
}

func verifyRemoteDBService(target, remotePath string, cfg *config.Config) error {
	projectRef := firstNonEmptyTrimmed(strings.TrimSpace(cfg.LastProjectSlug), strings.TrimSpace(cfg.LastProjectID))
	script := fmt.Sprintf(`cd %s && sudo systemctl is-active %s >/dev/null && (ss -ltn | grep ':%d ' || true) && pgrep -af 'db connect.*%s' || true`, shellQuote(remotePath), defaultRemoteDBServiceName, defaultRemoteDBListenPort, projectRef)
	out, err := runSSHScriptWithTimeout(target, script, 45*time.Second)
	if strings.TrimSpace(out) != "" {
		fmt.Println(strings.TrimSpace(out))
	}
	if err != nil {
		return fmt.Errorf("validación remota DB falló: %w", err)
	}
	return nil
}

func resolveVMRemote(cfg config.Config) (config.Config, string, string, error) {
	destination := normalizeMutagenDestinationForProject(strings.TrimSpace(cfg.MutagenDestination))
	if destination == "" {
		return cfg, "", "", fmt.Errorf("no hay mutagenDestination en config")
	}
	target, remotePath, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		return cfg, "", "", fmt.Errorf("no se pudo resolver destino SSH/path remoto de la VM")
	}
	return cfg, strings.TrimSpace(target), strings.TrimSpace(remotePath), nil
}

func loadVMTargetConfig() (config.Config, string, string, error) {
	cfg, err := config.Load()
	if err != nil {
		return config.Config{}, "", "", fmt.Errorf("no se pudo leer config local (.einar/config.json). Ejecuta init/login primero")
	}
	_, target, remotePath, err := resolveVMRemote(cfg)
	if err != nil {
		return config.Config{}, "", "", err
	}
	return cfg, target, remotePath, nil
}

func shellQuoteRemotePathPreserveTilde(v string) string {
	trimmed := strings.TrimSpace(v)
	if strings.HasPrefix(trimmed, "~/") {
		return "~/'" + strings.ReplaceAll(strings.TrimPrefix(trimmed, "~/"), "'", `"'"'"'`) + "'"
	}
	return shellQuote(trimmed)
}

func shellQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", `"'"'"'`) + "'" }
