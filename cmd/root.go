package cmd

import (
	"fmt"
	"os"

	"einarc/internal/config"

	"github.com/spf13/cobra"
)

var (
	apiURLFlag string
	tokenFlag  string
	jsonOutput bool
)

var rootCmd = &cobra.Command{
	Use:   "einarc",
	Short: "CLI de Einar",
	Long:  "einarc permite autenticarte y crear proyectos en Einar API.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	_ = config.LoadDotEnv(".env")

	rootCmd.PersistentFlags().StringVar(&apiURLFlag, "api-url", "", "Base URL de la API (fallback: EINAR_API_URL)")
	rootCmd.PersistentFlags().StringVar(&tokenFlag, "token", "", "PAT de Einar (fallback: EINAR_TOKEN)")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Salida JSON")
}
