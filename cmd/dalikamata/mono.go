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
		domainApp.DBPath = domainDBPath

		metricsApp := app.NewMetricsApp(l)
		metricsApp.NATSHost = natsURL
		metricsApp.NATSPort = natsPort
		metricsApp.MetricsURL = metricsAddr
		metricsApp.RefreshInterval = metricRefreshInterval
		metricsApp.AggregateTimeout = metricAggregateTimeout

		apiApp := app.NewAPIApp(l)
		apiApp.NATSHost = natsURL
		apiApp.NATSPort = natsPort
		apiApp.APIAddr = apiAddr
		apiApp.QueryTimeout = apiQueryTimeout

		ingestApp := app.NewIngestBitbucketApp(l)
		ingestApp.NATSHost = natsURL
		ingestApp.NATSPort = natsPort
		ingestApp.BitbucketURL = bitbucketURL
		ingestApp.BitbucketToken = bitbucketToken
		ingestApp.Projects = bitbucketProjects
		ingestApp.CACertsDir = caCertsDir
		ingestApp.Interval = bitbucketInterval
		ingestApp.ComponentConfigEnabled = bitbucketComponentCfgEnabled
		ingestApp.ComponentConfigFiles = bitbucketComponentCfgFiles

		var configApp *app.IngestConfigApp
		if componentsDir != "" {
			configApp = app.NewIngestConfigApp(l)
			configApp.NATSHost = natsURL
			configApp.NATSPort = natsPort
			configApp.Dir = componentsDir
		}

		var jenkinsApp *app.IngestJenkinsApp
		if jenkinsURL != "" {
			jenkinsApp = app.NewIngestJenkinsApp(l)
			jenkinsApp.NATSHost = natsURL
			jenkinsApp.NATSPort = natsPort
			jenkinsApp.JenkinsURL = jenkinsURL
			jenkinsApp.JenkinsUser = jenkinsUser
			jenkinsApp.JenkinsToken = jenkinsToken
			jenkinsApp.Jobs = jenkinsJobs
			jenkinsApp.RepoOverrides = jenkinsRepoOverride
			jenkinsApp.CACertsDir = caCertsDir
			jenkinsApp.Interval = jenkinsInterval
		}

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
			if err := domainApp.Run(ctx); err != nil {
				l.Error("running domain service", "error", err)
			}
		})
		wg.Go(func() {
			if err := metricsApp.Run(ctx); err != nil {
				l.Error("running metrics service", "error", err)
			}
		})
		wg.Go(func() {
			if err := apiApp.Run(ctx); err != nil {
				l.Error("running API service", "error", err)
			}
		})
		wg.Go(func() {
			if err := ingestApp.Run(ctx); err != nil {
				l.Error("running bitbucket ingest", "error", err)
			}
		})
		if configApp != nil {
			wg.Go(func() {
				if err := configApp.Run(ctx); err != nil {
					l.Error("running config ingest", "error", err)
				}
			})
		}
		if jenkinsApp != nil {
			wg.Go(func() {
				if err := jenkinsApp.Run(ctx); err != nil {
					l.Error("running jenkins ingest", "error", err)
				}
			})
		}

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
	addNATSFlags(monoCmd)
	addDomainFlags(monoCmd)
	addConfigFlags(monoCmd)
	addJenkinsFlags(monoCmd)
	addBitbucketFlags(monoCmd)
	addMetricsFlags(monoCmd)
	addApiFlags(monoCmd)
}
