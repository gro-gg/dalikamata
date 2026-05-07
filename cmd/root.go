package cmd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var (
	debugMode      bool
	natsHost       string
	natsPort       int
	natsPath       string
	withNatsServer bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "dalikamata",
	Short: "Collect, generate and evaluate Continuous Delivery Metrics in your Organization.",
	Long: `Collect, generate and evaluate Continuous Delivery Metrics in your Organization.

Dalikamata (or Dalikmata) is a pre-colonial Visayan deity of the Philippines, revered as the goddess of eyes,
eyesight, and clairvoyance. Known as a "many-eyed" deity, she is invoked to cure eye ailments, provide prophetic
visions, and is believed to shed tears of dew at night over human suffering.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		level := slog.LevelInfo
		if debugMode {
			level = slog.LevelDebug
		}
		// Use TextHandler for CLI output as it's easier for humans to read
		opts := &slog.HandlerOptions{Level: level}
		logger := slog.New(slog.NewTextHandler(os.Stdout, opts))

		// Set as default so third-party libs using slog pick it up
		slog.SetDefault(logger)
	},
}

func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Kill, os.Interrupt, syscall.SIGTERM)
	defer stop()
	rootCmd.SetContext(ctx)

	err := rootCmd.Execute()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "enable debug mode")
	rootCmd.PersistentFlags().BoolVar(&withNatsServer, "nats-server", true, "start NATS server?")
	rootCmd.PersistentFlags().StringVar(&natsHost, "nats-host", "0.0.0.0", "NATS server host name")
	rootCmd.PersistentFlags().IntVar(&natsPort, "nats-port", 4222, "NATS server port")
	rootCmd.PersistentFlags().StringVar(&natsPath, "nats-data", "./data/nats", "NATS server persistence path")
}
