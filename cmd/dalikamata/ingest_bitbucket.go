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
	bitbucketCmd.Flags().StringVar(&bitbucketURL, "bitbucket-url", "", "Bitbucket Server base URL (e.g. https://bitbucket.example.com)")
	bitbucketCmd.Flags().StringVar(&bitbucketToken, "bitbucket-token", "", "Bitbucket personal access token")
	bitbucketCmd.Flags().StringSliceVar(&bitbucketProjects, "bitbucket-projects", nil, "Bitbucket project keys to crawl (comma-separated)")
}
