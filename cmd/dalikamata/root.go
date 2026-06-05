package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/api"
	"codeberg.org/aeforged/dalikamata/internal/metrics"
	"github.com/spf13/cobra"
)

var (
	debugMode              bool
	natsURL                string // TODO rename to natsHost
	natsPort               int
	natsPath               string
	caCertsDir             string
	gracePeriod            time.Duration
	metricsAddr            string
	metricRefreshInterval  time.Duration
	metricAggregateTimeout time.Duration
	apiAddr                string
	apiQueryTimeout        time.Duration
	bitbucketURL           string
	bitbucketToken         string
	bitbucketProjects      []string
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

		return nil
	},
	SilenceUsage: true,
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

	os.Exit(0)
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "enable debug mode")
	rootCmd.PersistentFlags().StringVar(&natsURL, "nats-host", "0.0.0.0", "NATS server host name")
	rootCmd.PersistentFlags().IntVar(&natsPort, "nats-port", 4222, "NATS server port")
	rootCmd.PersistentFlags().StringVar(&caCertsDir, "ca-certs-dir", "", "directory containing custom CA certificates (.pem, .crt, .cer)")
	rootCmd.PersistentFlags().DurationVar(&gracePeriod, "grace-period", 10*time.Second, "grace period for shutdown")
	rootCmd.PersistentFlags().StringVar(&metricsAddr, "metrics-addr", metrics.DefaultMetricsAddr, "metrics HTTP listen address")
	rootCmd.PersistentFlags().DurationVar(&metricRefreshInterval, "metric-refresh-interval", metrics.DefaultRefreshInterval, "how often background loops recompute each metric")
	rootCmd.PersistentFlags().DurationVar(&metricAggregateTimeout, "metric-aggregate-timeout", metrics.DefaultAggregateTimeout, "per-aggregation query timeout for metric refresh loops")
	rootCmd.PersistentFlags().StringVar(&apiAddr, "api-addr", api.DefaultAPIAddr, "query API HTTP listen address")
	rootCmd.PersistentFlags().DurationVar(&apiQueryTimeout, "api-query-timeout", api.DefaultQueryTimeout, "per-request query timeout for the API server")
}
