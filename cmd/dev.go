package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"einarc/internal/config"

	"github.com/spf13/cobra"
)

var (
	devFollow bool
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Comandos de desarrollo remoto (Air) para el proyecto actual",
}

var devStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Inicia Air en la VM para el proyecto actual",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _, target, remotePath, err := resolveDevRemote()
		if err != nil {
			return err
		}
		if err := setupAndStartRemoteAir(&cfg); err != nil {
			return err
		}
		fmt.Printf("✅ Air iniciado en %s:%s\n", target, remotePath)
		return nil
	},
}

var devStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Detiene Air remoto para el proyecto actual",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _, target, remotePath, err := resolveDevRemote()
		if err != nil {
			return err
		}
		stopCmd := fmt.Sprintf(`cd %q && if [ -f .air.pid ] && kill -0 "$(cat .air.pid)" 2>/dev/null; then kill "$(cat .air.pid)" && rm -f .air.pid && echo "air detenido"; else echo "air no estaba corriendo"; fi`, remotePath)
		out, err := runSSHScript(target, stopCmd)
		if strings.TrimSpace(out) != "" {
			fmt.Println(strings.TrimSpace(out))
		}
		return err
	},
}

var devStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Muestra estado de Air remoto",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _, target, remotePath, err := resolveDevRemote()
		if err != nil {
			return err
		}
		statusCmd := fmt.Sprintf(`cd %q && if [ -f .air.pid ] && kill -0 "$(cat .air.pid)" 2>/dev/null; then echo "running pid=$(cat .air.pid)"; else echo "stopped"; fi`, remotePath)
		out, err := runSSHScript(target, statusCmd)
		if strings.TrimSpace(out) != "" {
			fmt.Println(strings.TrimSpace(out))
		}
		return err
	},
}

var devLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Muestra logs de Air remoto",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _, target, remotePath, err := resolveDevRemote()
		if err != nil {
			return err
		}
		logCmd := fmt.Sprintf("cd %q && touch .air.log && tail %s -n 200 .air.log", remotePath, map[bool]string{true: "-f", false: ""}[devFollow])
		c := exec.Command("ssh", target, "bash", "-s")
		c.Stdin = strings.NewReader(logCmd)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

func resolveDevRemote() (config.Config, string, string, string, error) {
	cfg, err := config.Load()
	if err != nil {
		return config.Config{}, "", "", "", fmt.Errorf("no se pudo leer config local (.einar/config.json). Ejecuta init/login primero")
	}
	if err := ensureWorkspaceBranchLock(&cfg); err != nil {
		return config.Config{}, "", "", "", err
	}
	if err := ensureWorkspaceOwnership(&cfg, false); err != nil {
		return config.Config{}, "", "", "", err
	}
	destination := strings.TrimSpace(cfg.MutagenDestination)
	if destination == "" {
		return cfg, "", "", "", fmt.Errorf("no hay mutagenDestination en config; ejecuta init en este proyecto")
	}
	destination = normalizeMutagenDestinationForProject(destination)
	target, remotePath, ok := sshTargetAndPathFromMutagenDestination(destination)
	if !ok {
		return cfg, destination, "", "", fmt.Errorf("destino mutagen inválido: %s", destination)
	}
	return cfg, destination, target, remotePath, nil
}

func init() {
	devLogsCmd.Flags().BoolVarP(&devFollow, "follow", "f", false, "Seguir logs en tiempo real")
	devCmd.AddCommand(devStartCmd)
	devCmd.AddCommand(devStopCmd)
	devCmd.AddCommand(devStatusCmd)
	devCmd.AddCommand(devLogsCmd)
	rootCmd.AddCommand(devCmd)
}
