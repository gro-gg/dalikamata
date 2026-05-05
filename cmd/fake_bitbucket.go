package cmd

import (
	"log/slog"

	"codeberg.org/aeforged/dalikamata/internal/ingest/bitbucket/fakeserver"
	"github.com/spf13/cobra"
)

var fakeBitbucketCmd = &cobra.Command{
	Use:   "bitbucket",
	Short: "start a fake Bitbucket Server for development and testing",
	Long: `start a fake Bitbucket Server for development and testing

The fake server exposes the Bitbucket Server REST API v1 with a small set of
pre-populated projects, repositories, commits, and pull requests.

Pre-populated projects:
  PROJ  – Project Alpha  (repos: backend-api, frontend-app, shared-lib)
  INFRA – Infrastructure (repos: k8s-configs, terraform-modules)

Point the ingest command at it with:
  dalikamata ingest bitbucket \
    --bitbucket-url http://localhost:7990 \
    --bitbucket-token any-token \
    --projects PROJ,INFRA`,
	Run: func(cmd *cobra.Command, args []string) {
		addr, _ := cmd.Flags().GetString("addr")

		srv := fakeserver.New(addr, slog.Default().With("service", "fake-bitbucket"))
		if err := srv.Start(cmd.Context()); err != nil {
			slog.Error("fake bitbucket server error", "error", err)
		}
	},
}

func init() {
	fakeCmd.AddCommand(fakeBitbucketCmd)

	fakeBitbucketCmd.Flags().String("addr", "localhost:7990", "address to listen on (host:port)")
}
