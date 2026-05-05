package cmd

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"codeberg.org/aeforged/dalikamata/internal/app"
)

// domainCmd represents the storage command
var domainCmd = &cobra.Command{
	Use:   "domain",
	Short: "start the domain service",
	Long:  `start the domain service`,
	RunE: func(cmd *cobra.Command, args []string) error {
		domainApp := app.NewDomainApp()

		var err error
		natsPort, err = cmd.PersistentFlags().GetInt("nats-port")
		if err == nil && natsPort != 0 {
			domainApp.NATSPort = natsPort
		}
		natsHost, err = cmd.PersistentFlags().GetString("nats-host")
		if err == nil && natsHost != "" {
			domainApp.NATSHost = natsHost
		}
		natsPath, err = cmd.PersistentFlags().GetString("nats-path")
		if err == nil && natsPath != "" {
			domainApp.DataDir = natsPath
		}
		withNATSServer, err := cmd.PersistentFlags().GetBool("nats-server")
		if err == nil {
			domainApp.WithNATSServer = withNATSServer
		}

		err = domainApp.Run(cmd.Root().Context(), slog.Default())
		if err != nil {
			return fmt.Errorf("starting domain service: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(domainCmd)
}
