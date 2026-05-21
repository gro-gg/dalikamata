package app

import (
	"context"
	"log/slog"

	"codeberg.org/aeforged/dalikamata/internal/nats"
	"codeberg.org/aeforged/dalikamata/internal/metrics"
	metricnats "codeberg.org/aeforged/dalikamata/internal/metrics/nats"
)

type MetricsApp struct {
	NATSHost   string
	NATSPort   int
	MetricsURL string
	logger     *slog.Logger
}

func NewMetricsApp(logger *slog.Logger) *MetricsApp {
	return &MetricsApp{
		NATSHost:   nats.DefaultHost,
		NATSPort:   nats.DefaultPort,
		MetricsURL: metrics.DefaultMetricsAddr,
		logger:     logger.With("service", "metrics"),
	}
}

func (a *MetricsApp) Run(ctx context.Context) error {
	port := metricnats.NewPort(
		a.logger.With("port", "nats"),
		nats.NATSConnectionString(a.NATSHost, a.NATSPort),
	)
	svc := metrics.NewMetricsService(port, a.logger, a.MetricsURL)
	return svc.Run(ctx)
}
