package app

import (
	"context"
	"fmt"
	"log/slog"

	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/ingest/bitbucket"
)

type IngestBitbucketApp struct {
	BitbucketURL   string
	BitbucketToken string
	NATSHost       string
	NATSPort       int
	Projects       []string
}

func NewIngestBitbucketApp() *IngestBitbucketApp {
	a := &IngestBitbucketApp{
		BitbucketURL:   "localhost:7999",
		BitbucketToken: "aToken",
		NATSHost:       dalinats.DefaultHost,
		NATSPort:       dalinats.DefaultPort,
		Projects:       []string{},
	}

	return a
}

func (a *IngestBitbucketApp) Run(ctx context.Context, logger *slog.Logger) error {
	l := logger.With("service", "ingest_bitbucket")
	natsURL := dalinats.NATSConnectionString(a.NATSHost, a.NATSPort)
	publisher, publisherCloser, err := dalinats.NewPublisher(ctx, natsURL, logger)
	if err != nil {
		return fmt.Errorf("create publisher: %w", err)
	}
	defer publisherCloser()

	l.Info("Starten Publisher", "nats_url", natsURL)
	client := bitbucket.NewClient(a.BitbucketURL, a.BitbucketToken, logger)
	crawler := bitbucket.NewCrawler(client, publisher, a.Projects, logger)

	bitbucketService, err := bitbucket.NewIngestBitbucketService(l, crawler)
	if err != nil {
		return fmt.Errorf("creating ingest bitbucket service: %w", err)
	}

	err = bitbucketService.Run(ctx)
	if err != nil {
		return fmt.Errorf("starting ingest bitbucket service: %w", err)
	}

	<-ctx.Done()

	return nil
}
