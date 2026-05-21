package main

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"codeberg.org/aeforged/dalikamata/internal/app"
)

var natsCmd = &cobra.Command{
	Use:   "nats",
	Short: "start the NATS server",
	Long:  `start the embedded NATS JetStream server`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Root().Context()

		natsApp := app.NewNATSApp(slog.Default())
		natsApp.Host = natsURL
		natsApp.Port = natsPort
		natsApp.DataDir = natsPath

		var wg sync.WaitGroup
		var runErr error

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := natsApp.Run(ctx); err != nil {
				slog.Default().Error("running NATS", "error", err)
				runErr = err
			}
		}()

		if err := natsApp.WaitForStartup(); err != nil {
			return fmt.Errorf("NATS startup: %w", err)
		}
		slog.Default().Info("NATS service running")

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
	rootCmd.AddCommand(natsCmd)
	natsCmd.Flags().StringVar(&natsPath, "nats-data", "./data/nats", "NATS server persistence path")
}
