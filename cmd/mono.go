package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"codeberg.org/aeforged/dalikamata/internal/app"
)

var monoCmd = &cobra.Command{
	Use:   "mono",
	Short: "start domain, ingest and metrics services together",
	Long:  `start domain, ingest and metrics services together`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if bitbucketURL == "" {
			return fmt.Errorf("--bitbucket-url is required")
		}
		if bitbucketToken == "" {
			return fmt.Errorf("--bitbucket-token is required")
		}

		domainApp := app.NewDomainApp(slog.Default())
		domainApp.NATSHost = natsURL
		domainApp.NATSPort = natsPort
		domainApp.DataDir = natsPath
		domainApp.WithNATSServer = true

		metricsApp := app.NewMetricsApp(slog.Default())
		metricsApp.NATSHost = natsURL
		metricsApp.NATSPort = natsPort
		metricsApp.MetricsURL = metricsAddr

		ingestApp := app.NewIngestBitbucketApp(slog.Default())
		ingestApp.NATSHost = natsURL
		ingestApp.NATSPort = natsPort
		ingestApp.BitbucketURL = bitbucketURL
		ingestApp.BitbucketToken = bitbucketToken
		ingestApp.Projects = bitbucketProjects
		ingestApp.CACertsDir = caCertsDir

		ctx := cmd.Root().Context()
		var wg sync.WaitGroup
		var (
			domainErr  error
			metricsErr error
			ingestErr  error
		)

		wg.Go(func() { domainErr = domainApp.Run(ctx) })
		wg.Go(func() { metricsErr = metricsApp.Run(ctx) })
		wg.Go(func() { ingestErr = ingestApp.Run(ctx) })

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
			return errors.Join(domainErr, metricsErr, ingestErr)
		}
	},
}

func init() {
	rootCmd.AddCommand(monoCmd)
}
