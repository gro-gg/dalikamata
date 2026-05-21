package main

import (
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "dalifakes",
	Short: "start a fake external service for development and testing",
	Long: `start a fake external service for development and testing

See the subcommands for more details.`,
}
