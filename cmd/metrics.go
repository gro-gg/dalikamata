package cmd

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"codeberg.org/aeforged/dalikamata/internal/app"
	"codeberg.org/aeforged/dalikamata/internal/metrics"
)

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Calculate and expose metrics",
	RunE: func(cmd *cobra.Command, args []string) error {
		metricsApp := app.NewMetricsApp()

		metricsURL, err := cmd.Flags().GetString("metrics-addr")
		if err == nil {
			metricsApp.MetricsURL = metricsURL
		}
		natsHost, err := cmd.Flags().GetString("nats-host")
		if err == nil {
			metricsApp.NATSHost = natsHost
		}
		natsPort, err := cmd.Flags().GetInt("nats-port")
		if err == nil {
			metricsApp.NATSPort = natsPort
		}

		err = metricsApp.Run(cmd.Root().Context(), slog.Default())
		if err != nil {
			return fmt.Errorf("running metrics app: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(metricsCmd)
	metricsCmd.Flags().String("metrics-addr", metrics.DefaultMetricsAddr, "metrics HTTP listen address")
}
