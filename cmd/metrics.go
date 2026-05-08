package cmd

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"codeberg.org/aeforged/dalikamata/internal/app"
)

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Calculate and expose metrics",
	RunE: func(cmd *cobra.Command, args []string) error {
		metricsApp := app.NewMetricsApp(slog.Default())

		metricsApp.MetricsURL = metricsAddr
		metricsApp.NATSHost = natsURL
		metricsApp.NATSPort = natsPort
		ctx := cmd.Root().Context()
		var wg sync.WaitGroup
		var runErr error

		wg.Go(func() {
			runErr = metricsApp.Run(ctx)
		})

		<-ctx.Done()

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-time.After(gracePeriod):
			return fmt.Errorf("shutdown grace period expired")
		case <-done:
			return runErr
		}
	},
}

func init() {
	rootCmd.AddCommand(metricsCmd)
}
