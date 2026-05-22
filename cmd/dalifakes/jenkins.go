package main

import (
	"log/slog"

	jenkinsfake "codeberg.org/aeforged/dalikamata/internal/ingest/jenkins/fakeserver"
	"github.com/spf13/cobra"
)

var dalifakesJenkinsCmd = &cobra.Command{
	Use:   "jenkins",
	Short: "start a fake Jenkins server for development and testing",
	Long: `start a fake Jenkins server for development and testing

The fake server exposes the Jenkins REST API and Pipeline Stage API with a small
set of pre-populated jobs and builds.

Pre-populated jobs:
  build-backend  (3 builds: SUCCESS, FAILURE, SUCCESS — 3 stages each)
  build-frontend (3 builds: SUCCESS, SUCCESS, FAILURE — 3 stages each)

Point the ingest command at it with:
  dalikamata ingest jenkins \
    --jenkins-url http://localhost:7991 \
    --jenkins-user any \
    --jenkins-token any`,
	Run: func(cmd *cobra.Command, args []string) {
		addr, _ := cmd.Flags().GetString("addr")
		srv := jenkinsfake.New(addr, slog.Default().With("service", "fake-jenkins"))
		if err := srv.Start(cmd.Context()); err != nil {
			slog.Error("fake jenkins server error", "error", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(dalifakesJenkinsCmd)
	dalifakesJenkinsCmd.Flags().String("addr", "localhost:7991", "address to listen on (host:port)")
}
