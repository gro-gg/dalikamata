package app

import (
	"context"
	"fmt"
	"log/slog"

	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/metrics"
	metricnats "codeberg.org/aeforged/dalikamata/internal/metrics/nats"
)

type MetricsApp struct {
	NATSHost   string
	NATSPort   int
	MetricsURL string
}

func NewMetricsApp() *MetricsApp {
	return &MetricsApp{
		NATSHost:   dalinats.DefaultHost,
		NATSPort:   dalinats.DefaultPort,
		MetricsURL: metrics.DefaultMetricsAddr,
	}
}

func (a *MetricsApp) Run(ctx context.Context, logger *slog.Logger) error {
	l := logger.With("service", "metrics")
	port := metricnats.NewPort(l.With("port", "nats"), dalinats.NATSConnectionString(a.NATSHost, a.NATSPort))
	svc := metrics.NewMetricsService(port, l, a.MetricsURL)

	shutdown, err := svc.Run(ctx)
	if err != nil {
		return fmt.Errorf("metrics service: %w", err)
	}

	<-ctx.Done()

	shutdown()

	return nil
}
