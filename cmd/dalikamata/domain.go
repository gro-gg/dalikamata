package main

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
		ctx := cmd.Root().Context()

		domainApp := app.NewDomainApp(slog.Default())
		domainApp.NATSHost = natsURL
		domainApp.NATSPort = natsPort
		var wg sync.WaitGroup
		var runErr error

		wg.Go(func() {
			err := domainApp.Run(ctx)
			if err != nil {
				slog.Default().Error("running domain app", "error", err)
			}
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
