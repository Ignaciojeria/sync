package cmd

import (
	"fmt"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ignaciojeria/sync/internal/config"

	"github.com/spf13/cobra"
)

const defaultRemoteCLIBinaryName = "sync"
const defaultRemoteCLIHomeBinDir = ".einar/bin"
const defaultRemoteDBServiceName = "einar-db-connect.service"
const defaultRemoteDBListenHost = "127.0.0.1"
const defaultRemoteDBListenPort = 15432

var vmAgentCmd = &cobra.Command{Use: "vm-agent", Short: "Bootstrap remoto del CLI en la VM"}

var vmAgentBootstrapDBCmd = &cobra.Command{
	Use:   "bootstrap-db",
	Short: "Resuelve/instala el CLI remoto en la VM y arranca sync db connect como servicio",
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
		remoteBinaryPath, _, err := resolveRemoteCLIBinaryPath(target, remotePath)
		if err != nil {
			return err
		}
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
	remoteBinaryPath, source, err := ensureRemoteCLIBinaryReady(target, remotePath)
	if err != nil {
		return err
	}
	fmt.Printf("✅ CLI remoto listo (%s): %s\n", source, remoteBinaryPath)
	if err := uploadRemoteProjectConfig(target, remotePath); err != nil {
		return err
	}
	if err := installRemoteDBService(target, remotePath, remoteBinaryPath, cfg); err != nil {
		return err
	}
	if err := verifyRemoteDBService(target, remotePath, cfg); err != nil {
		return err
	}
	fmt.Println("✅ Bootstrap remoto DB completado: CLI remoto resuelto y sync db connect corriendo en la VM")
	return nil
}

func buildAndUploadRemoteCLI(target, remotePath string) (string, error) {
	output := filepath.Join(".einar", "bin", "sync-linux-amd64")
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return "", err
	}
	repoRoot, err := resolveLocalCLISourceRoot()
	if err != nil {
		return "", err
	}
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return "", err
	}
	cmd := exec.Command("go", "build", "-o", absOutput, ".")
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go build CLI remoto falló: %w", err)
	}
	fmt.Printf("✅ CLI Linux compilado en %s\n", absOutput)
	remoteBinaryPath := strings.TrimRight(remotePath, "/") + "/" + defaultRemoteCLIBinaryName
	if _, err := runSSHScript(target, fmt.Sprintf("mkdir -p %s", shellQuote(remotePath))); err != nil {
		return "", fmt.Errorf("no se pudo preparar raíz remota del proyecto: %w", err)
	}
	scp := newSCPCommand(absOutput, target+":"+remoteBinaryPath)
	scp.Stdout = os.Stdout
	scp.Stderr = os.Stderr
	if err := scp.Run(); err != nil {
		return "", fmt.Errorf("scp CLI remoto falló: %w", err)
	}
	if _, err := runSSHScript(target, fmt.Sprintf("chmod +x %s", shellQuote(remoteBinaryPath))); err != nil {
		return "", fmt.Errorf("no se pudo marcar CLI remoto como ejecutable: %w", err)
	}
	fmt.Printf("✅ CLI subido a %s:%s\n", target, remoteBinaryPath)
	return remoteBinaryPath, nil
}

func ensureRemoteCLIBinaryReady(target, remotePath string) (string, string, error) {
	if remoteBinaryPath, source, err := resolveRemoteCLIBinaryPath(target, remotePath); err == nil {
		return remoteBinaryPath, source, nil
	}

	fmt.Println("ℹ️  CLI remoto no encontrado; intentando instalarlo en la VM con 'go install github.com/Ignaciojeria/sync@latest'...")
	if remoteBinaryPath, err := installRemoteCLIWithGoInstall(target); err == nil {
		return remoteBinaryPath, "go-install", nil
	}

	fmt.Println("ℹ️  Instalación remota falló; intentando fallback de build/upload local...")
	remoteBinaryPath, err := buildAndUploadRemoteCLI(target, remotePath)
	if err != nil {
		return "", "", fmt.Errorf("CLI remoto no disponible en la VM y fallback local falló: %w", err)
	}
	return remoteBinaryPath, "uploaded-from-local", nil
}

func installRemoteCLIWithGoInstall(target string) (string, error) {
	remoteBinaryPath := "$HOME/" + defaultRemoteCLIHomeBinDir + "/" + defaultRemoteCLIBinaryName
	script := fmt.Sprintf(`set -eu
if ! command -v go >/dev/null 2>&1; then
  echo "missing-go"
  exit 21
fi
go env -w GOPROXY=direct GOSUMDB=off
mkdir -p "$HOME/%s"
GOBIN="$HOME/%s" GOPROXY=direct GOSUMDB=off go install github.com/Ignaciojeria/sync@latest >/dev/null
chmod +x "%s"
printf '%%s\n' "%s"
`, defaultRemoteCLIHomeBinDir, defaultRemoteCLIHomeBinDir, remoteBinaryPath, remoteBinaryPath)
	out, err := runSSHScriptWithTimeout(target, script, 4*time.Minute)
	if err != nil {
		msg := strings.TrimSpace(out)
		if strings.Contains(msg, "missing-go") {
			return "", fmt.Errorf("la VM no tiene Go instalado para ejecutar 'go install github.com/Ignaciojeria/sync@latest'")
		}
		if msg != "" {
			return "", fmt.Errorf("go install remoto falló: %s", msg)
		}
		return "", fmt.Errorf("go install remoto falló: %w", err)
	}
	resolved := ""
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			resolved = trimmed
		}
	}
	if resolved == "" {
		return "", fmt.Errorf("go install remoto no devolvió ruta del binario")
	}
	return resolved, nil
}

func resolveRemoteCLIBinaryPath(target, remotePath string) (string, string, error) {
	remoteHomeCLIPath := "$HOME/" + defaultRemoteCLIHomeBinDir + "/" + defaultRemoteCLIBinaryName
	remoteWorkspaceCLIPath := strings.TrimRight(strings.TrimSpace(remotePath), "/") + "/" + defaultRemoteCLIBinaryName
	script := fmt.Sprintf(`set -eu
if [ -x "%s" ]; then
  printf '%%s|%%s' "%s" home-bin
  exit 0
fi
if [ -x %s ]; then
  printf '%%s|%%s' %s workspace-bin
  exit 0
fi
exit 1
`, remoteHomeCLIPath, remoteHomeCLIPath, shellQuoteRemotePathPreserveTilde(remoteWorkspaceCLIPath), shellQuoteRemotePathPreserveTilde(remoteWorkspaceCLIPath))
	out, err := runSSHScriptWithTimeout(target, script, 20*time.Second)
	if err != nil {
		return "", "", fmt.Errorf("no se encontró CLI remoto administrado por sync en la VM")
	}
	parts := strings.SplitN(strings.TrimSpace(out), "|", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("respuesta inválida resolviendo CLI remoto: %q", strings.TrimSpace(out))
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
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
	scp := newSCPCommand(localConfigPath, target+":"+remoteConfigPath)
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

func installRemoteDBService(target, remotePath, remoteBinaryPath string, cfg *config.Config) error {
	projectRef := firstNonEmptyTrimmed(strings.TrimSpace(cfg.LastProjectSlug), strings.TrimSpace(cfg.LastProjectID))
	if projectRef == "" {
		return fmt.Errorf("falta lastProjectSlug/lastProjectID en config local")
	}
	remoteRoot := strings.TrimRight(strings.TrimSpace(remotePath), "/")
	serviceUser := sshUserFromTarget(target)
	service := fmt.Sprintf(`[Unit]
Description=Einar DB tunnel via CLI
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=%s
WorkingDirectory=%s
ExecStart=%s db connect --project %s --host %s --port %d
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, serviceUser, remoteRoot, remoteBinaryPath, projectRef, defaultRemoteDBListenHost, defaultRemoteDBListenPort)
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
	script := fmt.Sprintf(`set -eu
cd %s
for i in 1 2 3 4 5 6 7 8 9 10; do
  if sudo systemctl is-active %s >/dev/null 2>&1 && ss -ltn | grep -q '127.0.0.1:%d'; then
    pgrep -af 'db connect.*%s' || true
    exit 0
  fi
  sleep 1
done
sudo systemctl status %s --no-pager -l || true
printf '\n---\n'
sudo journalctl -u %s -n 50 --no-pager || true
exit 1`, shellQuote(remotePath), defaultRemoteDBServiceName, defaultRemoteDBListenPort, projectRef, defaultRemoteDBServiceName, defaultRemoteDBServiceName)
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

func runProjectMigrationsOnVM(cfg config.Config, migrationsDir, explicitDatabaseURL string) error {
	migrationsDir = strings.TrimSpace(migrationsDir)
	if migrationsDir == "" {
		return fmt.Errorf("carpeta de migraciones vacía")
	}
	stat, err := os.Stat(migrationsDir)
	if err != nil {
		return fmt.Errorf("carpeta de migraciones no encontrada en %s: %w", migrationsDir, err)
	}
	if !stat.IsDir() {
		return fmt.Errorf("ruta de migraciones no es carpeta: %s", migrationsDir)
	}

	_, target, remotePath, err := resolveVMRemote(cfg)
	if err != nil {
		return err
	}

	localMigrationsDir, err := filepath.Abs(migrationsDir)
	if err != nil {
		return fmt.Errorf("no se pudo resolver ruta absoluta de migraciones: %w", err)
	}

	remoteRoot := strings.TrimRight(strings.TrimSpace(remotePath), "/")
	if remoteRoot == "" {
		return fmt.Errorf("ruta remota de workspace vacía")
	}
	remoteMigrationsDir := remoteRoot + "/" + strings.TrimLeft(filepath.ToSlash(migrationsDir), "/")
	remoteParentDir := pathpkg.Dir(remoteMigrationsDir)

	prepareScript := fmt.Sprintf("mkdir -p %s && rm -rf %s", shellQuote(remoteParentDir), shellQuote(remoteMigrationsDir))
	if out, err := runSSHScriptWithTimeout(target, prepareScript, 30*time.Second); err != nil {
		msg := strings.TrimSpace(out)
		if msg != "" {
			return fmt.Errorf("no se pudo preparar carpeta remota de migraciones: %w\n%s", err, msg)
		}
		return fmt.Errorf("no se pudo preparar carpeta remota de migraciones: %w", err)
	}

	scp := newSCPCommand("-r", localMigrationsDir, target+":"+remoteParentDir)
	scp.Stdout = os.Stdout
	scp.Stderr = os.Stderr
	if err := scp.Run(); err != nil {
		return fmt.Errorf("scp de migraciones a la VM falló: %w", err)
	}
	fmt.Printf("✅ Migraciones subidas a %s:%s\n", target, remoteMigrationsDir)

	databaseURL := strings.TrimSpace(explicitDatabaseURL)
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(cfg.ProjectDatabaseURL)
	}
	if databaseURL == "" {
		return fmt.Errorf("falta DATABASE_URL del proyecto")
	}
	remoteDatabaseURL, err := localDatabaseURL(databaseURL, defaultRemoteDBListenHost, defaultRemoteDBListenPort)
	if err != nil {
		return fmt.Errorf("no se pudo adaptar DATABASE_URL al túnel DB remoto: %w", err)
	}

	remoteBinaryPath, _, err := resolveRemoteCLIBinaryPath(target, remotePath)
	if err != nil {
		return err
	}
	remoteCmd := fmt.Sprintf("cd %s && %s db migrate --dir %s --database-url %s --no-ssh-forward", shellQuote(remoteRoot), shellQuote(remoteBinaryPath), shellQuote(migrationsDir), shellQuote(remoteDatabaseURL))
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		out, err := runSSHScriptWithTimeout(target, remoteCmd, 90*time.Second)
		if strings.TrimSpace(out) != "" {
			fmt.Println(strings.TrimSpace(out))
		}
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt < 5 {
			fmt.Printf("ℹ️  Migraciones intento %d/5 falló; esperando túnel DB remoto...\n", attempt)
			time.Sleep(3 * time.Second)
		}
	}
	return fmt.Errorf("ejecución remota de migraciones falló: %w", lastErr)
}

func sshUserFromTarget(target string) string {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "exedev"
	}
	if strings.Contains(trimmed, "@") {
		parts := strings.SplitN(trimmed, "@", 2)
		if user := strings.TrimSpace(parts[0]); user != "" {
			return user
		}
	}
	return "exedev"
}

func shellQuoteRemotePathPreserveTilde(v string) string {
	trimmed := strings.TrimSpace(v)
	if strings.HasPrefix(trimmed, "~/") {
		return "~/'" + strings.ReplaceAll(strings.TrimPrefix(trimmed, "~/"), "'", `"'"'"'`) + "'"
	}
	return shellQuote(trimmed)
}

func shellQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", `"'"'"'`) + "'" }
