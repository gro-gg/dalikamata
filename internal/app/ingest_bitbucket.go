package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/httpclient"
	"codeberg.org/aeforged/dalikamata/internal/ingest/bitbucket"
	internalnats "codeberg.org/aeforged/dalikamata/internal/nats"
)

const (
	defaultBitbucketInterval  = 5 * time.Minute
	bitbucketCursorBucketName = "ingest-bitbucket-cursors"
)

type IngestBitbucketApp struct {
	BitbucketURL   string
	BitbucketToken string
	NATSHost       string
	NATSPort       int
	Projects       []string
	CACertsDir     string
	Interval       time.Duration
	logger         *slog.Logger
}

func NewIngestBitbucketApp(logger *slog.Logger) *IngestBitbucketApp {
	a := &IngestBitbucketApp{
		BitbucketURL:   "localhost:7999",
		BitbucketToken: "aToken",
		NATSHost:       internalnats.DefaultHost,
		NATSPort:       internalnats.DefaultPort,
		Projects:       []string{},
		Interval:       defaultBitbucketInterval,
		logger:         logger.With("service", "ingest_bitbucket"),
	}

	return a
}

func (a *IngestBitbucketApp) Run(ctx context.Context) error {
	natsURL := internalnats.NATSConnectionString(a.NATSHost, a.NATSPort)

	publisher, publisherCloser, err := dalinats.NewGitPublisher(ctx, natsURL, a.logger.With("port", "domain", "connection", "nats"))
	if err != nil {
		return fmt.Errorf("create publisher: %w", err)
	}
	defer publisherCloser()

	_, js, jsCloser, err := internalnats.Connect(ctx, natsURL, a.logger.With("connection", "nats", "client", "cursor-store"))
	if err != nil {
		return fmt.Errorf("connecting to NATS for cursor store: %w", err)
	}
	defer jsCloser()

	cursors, err := bitbucket.NewJetStreamCursors(ctx, js, bitbucketCursorBucketName)
	if err != nil {
		return fmt.Errorf("creating cursor store: %w", err)
	}

	a.logger.Info("Starten Publisher", "nats_url", natsURL)
	httpCl, err := httpclient.NewHTTPClient(a.CACertsDir)
	if err != nil {
		return fmt.Errorf("building HTTP client: %w", err)
	}
	client := bitbucket.NewClient(a.BitbucketURL, a.BitbucketToken, httpCl, a.logger)
	crawler := bitbucket.NewCrawler(client, publisher, cursors, a.Projects, a.logger)

	bitbucketService, err := bitbucket.NewIngestBitbucketService(crawler, a.Interval, a.logger)
	if err != nil {
		return fmt.Errorf("creating ingest bitbucket service: %w", err)
	}

	err = bitbucketService.Run(ctx)
	if err != nil {
		return fmt.Errorf("starting ingest bitbucket service: %w", err)
	}

	return nil
}
