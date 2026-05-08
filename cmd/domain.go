package cmd

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"codeberg.org/aeforged/dalikamata/internal/app"
)

// domainCmd represents the storage command
var domainCmd = &cobra.Command{
	Use:   "domain",
	Short: "start the domain service",
	Long:  `start the domain service`,
	RunE: func(cmd *cobra.Command, args []string) error {
		domainApp := app.NewDomainApp(slog.Default())

		if natsHost, err := cmd.PersistentFlags().GetString("nats-host"); err == nil && natsHost != "" {
			domainApp.NATSHost = natsHost
		}
		if natsPort, err := cmd.PersistentFlags().GetInt("nats-port"); err == nil && natsPort != 0 {
			domainApp.NATSPort = natsPort
		}
		if natsData, err := cmd.PersistentFlags().GetString("nats-data"); err == nil && natsData != "" {
			domainApp.DataDir = natsData
		}
		ctx := cmd.Root().Context()
		var wg sync.WaitGroup
		var runErr error

		wg.Go(func() {
			runErr = domainApp.Run(ctx)
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
	rootCmd.AddCommand(domainCmd)
}
