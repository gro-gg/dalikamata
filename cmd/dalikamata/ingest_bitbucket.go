package main

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/app"
	"github.com/spf13/cobra"
)

// bitbucketCmd represents the bitbucket command
var bitbucketCmd = &cobra.Command{
	Use:   "bitbucket",
	Short: "start a bitbucket data ingestion service",
	Long:  `start a bitbucket data ingestion service`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if bitbucketURL == "" {
			return fmt.Errorf("--bitbucket-url is required")
		}
		if bitbucketToken == "" {
			return fmt.Errorf("--bitbucket-token is required")
		}

		app := app.NewIngestBitbucketApp(slog.Default())
		app.NATSHost = natsURL
		app.NATSPort = natsPort
		app.BitbucketURL = bitbucketURL
		app.BitbucketToken = bitbucketToken
		app.Projects = bitbucketProjects
		app.CACertsDir = caCertsDir
		app.Interval = bitbucketInterval
		app.ComponentConfigEnabled = bitbucketComponentCfgEnabled
		app.ComponentConfigFiles = bitbucketComponentCfgFiles
		ctx := cmd.Root().Context()
		var wg sync.WaitGroup
		var runErr error

		wg.Go(func() {
			runErr = app.Run(ctx)
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
	ingestCmd.AddCommand(bitbucketCmd)
	addBitbucketFlags(bitbucketCmd)
}

func addBitbucketFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&bitbucketURL, "bitbucket-url", "", "Bitbucket Server base URL (e.g. https://bitbucket.example.com)")
	cmd.Flags().StringVar(&bitbucketToken, "bitbucket-token", "", "Bitbucket personal access token")
	cmd.Flags().StringSliceVar(&bitbucketProjects, "bitbucket-projects", nil, "Bitbucket project keys to crawl (comma-separated)")
	cmd.Flags().DurationVar(&bitbucketInterval, "bitbucket-interval", 5*time.Minute, "how often to re-crawl Bitbucket for new commits and pull requests")
	cmd.Flags().BoolVar(&bitbucketComponentCfgEnabled, "bitbucket-component-config-enabled", false, "enable per-repo self-onboarding: fetch an in-repo config file from each repo root")
	cmd.Flags().StringSliceVar(&bitbucketComponentCfgFiles, "bitbucket-component-config-file", []string{"dalikamata.yaml", "dalikamata.yml", ".dalikamata.yaml", ".dalikamata.yml"}, "in-repo config path(s) tried per repo for self-onboarding, first match wins (comma-separated; requires --bitbucket-component-config-enabled)")
}
