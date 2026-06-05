package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/api"
	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/nats"
)

// APIApp wires a dalinats.QueryClient to an api.Server and runs them together.
type APIApp struct {
	NATSHost     string
	NATSPort     int
	APIAddr      string
	QueryTimeout time.Duration
	logger       *slog.Logger
}

func NewAPIApp(logger *slog.Logger) *APIApp {
	return &APIApp{
		NATSHost:     nats.DefaultHost,
		NATSPort:     nats.DefaultPort,
		APIAddr:      api.DefaultAPIAddr,
		QueryTimeout: api.DefaultQueryTimeout,
		logger:       logger.With("service", "api"),
	}
}

func (a *APIApp) Run(ctx context.Context) error {
	nc, _, closeConn, err := nats.Connect(ctx, nats.NATSConnectionString(a.NATSHost, a.NATSPort), a.logger)
	if err != nil {
		return fmt.Errorf("connecting to NATS: %w", err)
	}
	defer closeConn()

	client := dalinats.NewQueryClient(nc, a.logger.With("component", "query-client"),
		dalinats.WithQueryTimeout(a.QueryTimeout),
	)
	srv := api.NewServer(client, a.logger, a.APIAddr,
		api.WithQueryTimeout(a.QueryTimeout),
	)
	return srv.Run(ctx)
}
