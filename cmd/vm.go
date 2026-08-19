package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var vmCmd = &cobra.Command{
	Use:   "vm",
	Short: "Ejecuta comandos en la VM del proyecto actual",
}

var vmExecCmd = &cobra.Command{
	Use:   "exec -- <command>",
	Short: "Sincroniza cambios y ejecuta un comando en la VM",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, target, remotePath, err := loadVMTargetConfig()
		if err != nil {
			return err
		}
		if err := ensureMutagenYAMLForWorkspace(&cfg); err != nil {
			return err
		}
		mutagenBin, err := resolveMutagenBinary()
		if err != nil {
			return err
		}
		if err := startAndFlushMutagenProject(mutagenBin, &cfg); err != nil {
			return fmt.Errorf("no se pudo sincronizar antes de ejecutar en VM: %w", err)
		}

		out, err := runSSHScriptWithTimeout(target, "cd "+shellQuote(remotePath)+"\n"+strings.Join(args, " "), 15*time.Minute)
		if out != "" {
			fmt.Println(out)
		}
		return err
	},
}

func init() {
	vmCmd.AddCommand(vmExecCmd)
	rootCmd.AddCommand(vmCmd)
}
