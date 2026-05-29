package app

import (
	"context"
	"fmt"
	"log/slog"

	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	ingestconfig "codeberg.org/aeforged/dalikamata/internal/ingest/config"
	"codeberg.org/aeforged/dalikamata/internal/nats"
)

type IngestConfigApp struct {
	Dir      string
	NATSHost string
	NATSPort int
	logger   *slog.Logger
}

func NewIngestConfigApp(logger *slog.Logger) *IngestConfigApp {
	return &IngestConfigApp{
		NATSHost: nats.DefaultHost,
		NATSPort: nats.DefaultPort,
		logger:   logger.With("service", "ingest_config"),
	}
}

func (a *IngestConfigApp) Run(ctx context.Context) error {
	natsURL := nats.NATSConnectionString(a.NATSHost, a.NATSPort)
	publisher, publisherCloser, err := dalinats.NewPlatformPublisher(ctx, natsURL, a.logger.With("port", "domain", "connection", "nats"))
	if err != nil {
		return fmt.Errorf("create platform publisher: %w", err)
	}
	defer publisherCloser()

	crawler := ingestconfig.NewCrawler(publisher, a.Dir, a.logger)
	if err := crawler.Run(ctx); err != nil {
		return fmt.Errorf("running config crawler: %w", err)
	}
	return nil
}
