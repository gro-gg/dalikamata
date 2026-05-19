package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/app"
	"codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/metrics"
	"github.com/spf13/cobra"
)

var (
	debugMode         bool
	natsURL           string
	natsPort          int
	natsPath          string
	withNatsServer    bool
	caCertsDir        string
	gracePeriod       time.Duration
	metricsAddr       string
	bitbucketURL      string
	bitbucketToken    string
	bitbucketProjects []string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "dalikamata",
	Short: "Collect, generate and evaluate Continuous Delivery Metrics in your Organization.",
	Long: `Collect, generate and evaluate Continuous Delivery Metrics in your Organization.

Dalikamata (or Dalikmata) is a pre-colonial Visayan deity of the Philippines, revered as the goddess of eyes,
eyesight, and clairvoyance. Known as a "many-eyed" deity, she is invoked to cure eye ailments, provide prophetic
visions, and is believed to shed tears of dew at night over human suffering.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		level := slog.LevelInfo
		if debugMode {
			level = slog.LevelDebug
		}
		// Use TextHandler for CLI output as it's easier for humans to read
		opts := &slog.HandlerOptions{Level: level}
		logger := slog.New(slog.NewTextHandler(os.Stdout, opts))

		// Set as default so third-party libs using slog pick it up
		slog.SetDefault(logger)

		if withNatsServer {
			wg := app.WaitGroupFrom(cmd.Context())
			natsServer := nats.NewServer()
			natsServer.DataDir = natsPath
			natsServer.Port = natsPort
			natsServer.Host = natsURL
			wg.Go(func() {
				if err := natsServer.Start(cmd.Context()); err != nil {
					slog.Error("running NATS server", "error", err)
				}
			})
			if err := natsServer.WaitForStartup(); err != nil {
				return fmt.Errorf("waiting for NATS server: %w", err)
			}
		}

		return nil
	},
	SilenceUsage: true,
}

func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Kill, os.Interrupt, syscall.SIGTERM)
	defer stop()

	rootCmd.SetContext(app.AddWaitGroup(ctx))

	err := rootCmd.Execute()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "enable debug mode")
	rootCmd.PersistentFlags().BoolVar(&withNatsServer, "nats-server", true, "start NATS server")
	rootCmd.PersistentFlags().StringVar(&natsURL, "nats-host", "0.0.0.0", "NATS server host name")
	rootCmd.PersistentFlags().IntVar(&natsPort, "nats-port", 4222, "NATS server port")
	rootCmd.PersistentFlags().StringVar(&natsPath, "nats-data", "./data/nats", "NATS server persistence path")
	rootCmd.PersistentFlags().StringVar(&caCertsDir, "ca-certs-dir", "", "directory containing custom CA certificates (.pem, .crt, .cer)")
	rootCmd.PersistentFlags().DurationVar(&gracePeriod, "grace-period", 10*time.Second, "grace period for shutdown")
	rootCmd.PersistentFlags().StringVar(&metricsAddr, "metrics-addr", metrics.DefaultMetricsAddr, "metrics HTTP listen address")
}
