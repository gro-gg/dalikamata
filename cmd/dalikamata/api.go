package main

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/api"
	"github.com/spf13/cobra"

	"codeberg.org/aeforged/dalikamata/internal/app"
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Serve the HTTP query API",
	Long:  `Serve the HTTP JSON query API for raw entity access (requires a running NATS server).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiApp := app.NewAPIApp(slog.Default())
		apiApp.NATSHost = natsURL
		apiApp.NATSPort = natsPort
		apiApp.APIAddr = apiAddr
		apiApp.QueryTimeout = apiQueryTimeout

		ctx := cmd.Root().Context()
		var wg sync.WaitGroup
		var runErr error

		wg.Go(func() {
			runErr = apiApp.Run(ctx)
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
	rootCmd.AddCommand(apiCmd)
	addApiFlags(apiCmd)
}

func addApiFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&apiAddr, "api-addr", api.DefaultAPIAddr, "query API HTTP listen address")
	cmd.Flags().DurationVar(&apiQueryTimeout, "api-query-timeout", api.DefaultQueryTimeout, "per-request query timeout for the API server")
}
