package main

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"codeberg.org/aeforged/dalikamata/internal/app"
)

var componentsDir string

var configIngestCmd = &cobra.Command{
	Use:   "config",
	Short: "ingest component configuration files",
	Long: `Read per-component YAML files from a directory and publish Team and Component
events to the NATS platform ingest subjects.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if componentsDir == "" {
			return fmt.Errorf("--component-config-dir is required")
		}

		a := app.NewIngestConfigApp(slog.Default())
		a.NATSHost = natsURL
		a.NATSPort = natsPort
		a.Dir = componentsDir

		return a.Run(cmd.Root().Context())
	},
}

func init() {
	ingestCmd.AddCommand(configIngestCmd)
	addConfigFlags(configIngestCmd)
}

func addConfigFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&componentsDir, "component-config-dir", "", "directory containing component YAML files")
}
