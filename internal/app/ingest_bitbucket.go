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
	defaultBitbucketInterval   = 5 * time.Minute
	bitbucketCursorBucketName  = "ingest-bitbucket-cursors"
	defaultComponentConfigFile = "dalikamata.yaml"
)

type IngestBitbucketApp struct {
	BitbucketURL   string
	BitbucketToken string
	NATSHost       string
	NATSPort       int
	Projects       []string
	CACertsDir     string
	Interval       time.Duration
	// ComponentConfigEnabled turns on per-repo self-onboarding (ADR-007).
	// When set, ComponentConfigFile is fetched from each repo root.
	ComponentConfigEnabled bool
	// ComponentConfigFile is the in-repo path fetched per repo for
	// self-onboarding. Defaults to defaultComponentConfigFile.
	ComponentConfigFile string
	logger              *slog.Logger
}

func NewIngestBitbucketApp(logger *slog.Logger) *IngestBitbucketApp {
	a := &IngestBitbucketApp{
		BitbucketURL:        "localhost:7999",
		BitbucketToken:      "aToken",
		NATSHost:            internalnats.DefaultHost,
		NATSPort:            internalnats.DefaultPort,
		Projects:            []string{},
		Interval:            defaultBitbucketInterval,
		ComponentConfigFile: defaultComponentConfigFile,
		logger:              logger.With("service", "ingest_bitbucket"),
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

	var crawlerOpts []bitbucket.CrawlerOption
	if a.ComponentConfigEnabled {
		configFile := a.ComponentConfigFile
		if configFile == "" {
			configFile = defaultComponentConfigFile
		}
		platformPublisher, platformCloser, err := dalinats.NewPlatformPublisher(ctx, natsURL, a.logger.With("port", "domain", "connection", "nats", "publisher", "platform"))
		if err != nil {
			return fmt.Errorf("create platform publisher: %w", err)
		}
		defer platformCloser()
		crawlerOpts = append(crawlerOpts, bitbucket.WithComponentConfig(platformPublisher, configFile))
		a.logger.Info("per-repo self-onboarding enabled", "component_config", configFile)
	}

	crawler := bitbucket.NewCrawler(client, publisher, cursors, a.Projects, a.logger, crawlerOpts...)

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
