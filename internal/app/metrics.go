package app

import (
	"context"
	"fmt"
	"log/slog"

	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/metrics"
	"codeberg.org/aeforged/dalikamata/internal/nats"
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
	nc, _, closeConn, err := nats.Connect(ctx, nats.NATSConnectionString(a.NATSHost, a.NATSPort), a.logger)
	if err != nil {
		return fmt.Errorf("connecting to NATS: %w", err)
	}
	defer closeConn()

	querier := dalinats.NewQueryClient(nc, a.logger.With("component", "query-client"))
	svc := metrics.NewMetricsService(querier, a.logger, a.MetricsURL)
	return svc.Run(ctx)
}
