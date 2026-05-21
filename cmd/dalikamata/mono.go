package main

import (
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

		l := slog.Default()

		natsApp := app.NewNATSApp(l)
		natsApp.Host = natsURL
		natsApp.Port = natsPort
		natsApp.DataDir = natsPath

		domainApp := app.NewDomainApp(l)
		domainApp.NATSHost = natsURL
		domainApp.NATSPort = natsPort

		metricsApp := app.NewMetricsApp(l)
		metricsApp.NATSHost = natsURL
		metricsApp.NATSPort = natsPort
		metricsApp.MetricsURL = metricsAddr

		ingestApp := app.NewIngestBitbucketApp(l)
		ingestApp.NATSHost = natsURL
		ingestApp.NATSPort = natsPort
		ingestApp.BitbucketURL = bitbucketURL
		ingestApp.BitbucketToken = bitbucketToken
		ingestApp.Projects = bitbucketProjects
		ingestApp.CACertsDir = caCertsDir

		ctx := cmd.Root().Context()
		var wg sync.WaitGroup

		wg.Go(func() {
			if err := natsApp.Run(ctx); err != nil {
				l.Error("running NATS", "error", err)
			}
		})
		if err := natsApp.WaitForStartup(); err != nil {
			return fmt.Errorf("NATS startup: %w", err)
		}

		wg.Go(func() {
			domainErr := domainApp.Run(ctx)
			if domainErr != nil {
				l.Error("running domain service", "error", domainErr)
			}
		})
		wg.Go(func() {
			metricsErr := metricsApp.Run(ctx)
			if metricsErr != nil {
				l.Error("running domain service", "error", metricsErr)
			}
		})
		wg.Go(func() {
			ingestErr := ingestApp.Run(ctx)
			if ingestErr != nil {
				l.Error("running domain service", "error", ingestErr)
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
			return nil
		}
	},
}

func init() {
	rootCmd.AddCommand(monoCmd)
	monoCmd.Flags().StringVar(&natsPath, "nats-data", "./data/nats", "NATS server persistence path")
	monoCmd.Flags().StringVar(&bitbucketURL, "bitbucket-url", "", "Bitbucket Server base URL (e.g. https://bitbucket.example.com)")
	monoCmd.Flags().StringVar(&bitbucketToken, "bitbucket-token", "", "Bitbucket personal access token")
	monoCmd.Flags().StringSliceVar(&bitbucketProjects, "bitbucket-projects", nil, "Bitbucket project keys to crawl (comma-separated)")
}
