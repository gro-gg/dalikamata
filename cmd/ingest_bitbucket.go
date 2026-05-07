package cmd

import (
	"fmt"
	"log/slog"

	"codeberg.org/aeforged/dalikamata/internal/app"
	"github.com/spf13/cobra"
)

// bitbucketCmd represents the bitbucket command
var bitbucketCmd = &cobra.Command{
	Use:   "bitbucket",
	Short: "start a bitbucket data ingestion service",
	Long:  `start a bitbucket data ingestion service`,
	RunE: func(cmd *cobra.Command, args []string) error {
		app := app.NewIngestBitbucketApp()

		var err error
		natsHost, err = cmd.Flags().GetString("nats-host")
		if err == nil {
			app.NATSHost = natsHost
		}
		natsPort, err = cmd.Flags().GetInt("nats-port")
		if err == nil {
			app.NATSPort = natsPort
		}
		bbURL, err := cmd.Flags().GetString("bitbucket-url")
		if err == nil {
			app.BitbucketURL = bbURL
		}
		bbToken, err := cmd.Flags().GetString("bitbucket-token")
		if err == nil {
			app.BitbucketToken = bbToken
		}
		projects, err := cmd.Flags().GetStringSlice("bitbucket-projects")
		if err == nil {
			app.Projects = projects
		}
		caCertsDir, err = cmd.Flags().GetString("ca-certs-dir")
		if err == nil {
			app.CACertsDir = caCertsDir
		}

		err = app.Run(cmd.Root().Context(), slog.Default())
		if err != nil {
			return fmt.Errorf("starting ingest-bitbucket service: %w", err)
		}

		return nil
	},
}

func init() {
	ingestCmd.AddCommand(bitbucketCmd)

	bitbucketCmd.Flags().String("bitbucket-url", "", "Bitbucket Server base URL (e.g. https://bitbucket.example.com)")
	bitbucketCmd.Flags().String("bitbucket-token", "", "Bitbucket personal access token")
	bitbucketCmd.Flags().StringSlice("bitbucket-projects", nil, "Bitbucket project keys to crawl (comma-separated)")

	_ = bitbucketCmd.MarkFlagRequired("bitbucket-url")
	_ = bitbucketCmd.MarkFlagRequired("bitbucket-token")
}
