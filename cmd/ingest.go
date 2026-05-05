package cmd

import (
	"github.com/spf13/cobra"
)

// ingestCmd represents the ingest command
var ingestCmd = &cobra.Command{
	Use:   "ingest",
	Short: "start one of the data ingestion services",
	Long: `start one of the data ingestion services

See the subcommands for more details.`,
}

func init() {
	rootCmd.AddCommand(ingestCmd)
}
