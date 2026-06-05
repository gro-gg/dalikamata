package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/metrics"
	"codeberg.org/aeforged/dalikamata/internal/nats"
)

type MetricsApp struct {
	NATSHost        string
	NATSPort        int
	MetricsURL      string
	RefreshInterval time.Duration
	AggregateTimeout time.Duration
	logger          *slog.Logger
}

func NewMetricsApp(logger *slog.Logger) *MetricsApp {
	return &MetricsApp{
		NATSHost:         nats.DefaultHost,
		NATSPort:         nats.DefaultPort,
		MetricsURL:       metrics.DefaultMetricsAddr,
		RefreshInterval:  metrics.DefaultRefreshInterval,
		AggregateTimeout: metrics.DefaultAggregateTimeout,
		logger:           logger.With("service", "metrics"),
	}
}

func (a *MetricsApp) Run(ctx context.Context) error {
	nc, _, closeConn, err := nats.Connect(ctx, nats.NATSConnectionString(a.NATSHost, a.NATSPort), a.logger)
	if err != nil {
		return fmt.Errorf("connecting to NATS: %w", err)
	}
	defer closeConn()

	aggregator := dalinats.NewQueryClient(nc, a.logger.With("component", "query-client"),
		dalinats.WithQueryTimeout(a.AggregateTimeout),
	)
	svc := metrics.NewMetricsService(aggregator, a.logger, a.MetricsURL,
		metrics.WithRefreshInterval(a.RefreshInterval),
		metrics.WithAggregateTimeout(a.AggregateTimeout),
	)
	return svc.Run(ctx)
}
