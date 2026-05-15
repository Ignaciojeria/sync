package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	mutagenName        string
	mutagenSource      string
	mutagenDestination string
)

var mutagenCmd = &cobra.Command{
	Use:   "mutagen",
	Short: "Genera una configuración base de Mutagen",
	Long:  "Crea un archivo mutagen.yml con una sesión de sincronización base para incluir Mutagen en tu proyecto.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if mutagenDestination == "" {
			return fmt.Errorf("debes indicar --destination (ej: docker://mi-contenedor/var/www)")
		}

		if mutagenSource == "" {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			mutagenSource = wd
		}

		absSource, err := filepath.Abs(mutagenSource)
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
`, mutagenName, absSource, mutagenDestination)

		if _, err := os.Stat("mutagen.yml"); err == nil {
			return fmt.Errorf("ya existe mutagen.yml en este directorio")
		}

		if err := os.WriteFile("mutagen.yml", []byte(content), 0o644); err != nil {
			return err
		}

		fmt.Println("✅ mutagen.yml creado")
		fmt.Println("Siguiente paso: ejecuta 'mutagen project start'")
		return nil
	},
}

func init() {
	mutagenCmd.Flags().StringVar(&mutagenName, "name", "dev-sync", "Nombre de la sesión de sincronización")
	mutagenCmd.Flags().StringVar(&mutagenSource, "source", "", "Ruta local del proyecto (por defecto: directorio actual)")
	mutagenCmd.Flags().StringVar(&mutagenDestination, "destination", "", "Destino remoto (ej: docker://mi-contenedor/var/www)")

	rootCmd.AddCommand(mutagenCmd)
}
