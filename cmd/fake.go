package cmd

import (
	"github.com/spf13/cobra"
)

// fakeCmd is the parent command for fake services used in development and testing.
var fakeCmd = &cobra.Command{
	Use:   "fake",
	Short: "start a fake external service for development and testing",
	Long: `start a fake external service for development and testing

See the subcommands for more details.`,
}

func init() {
	rootCmd.AddCommand(fakeCmd)
}
