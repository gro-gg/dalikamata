package app

import (
	"context"
	"fmt"
	"log/slog"

	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/httpclient"
	"codeberg.org/aeforged/dalikamata/internal/ingest/jenkins"
	"codeberg.org/aeforged/dalikamata/internal/nats"
)

type IngestJenkinsApp struct {
	JenkinsURL   string
	JenkinsUser  string
	JenkinsToken string
	NATSHost     string
	NATSPort     int
	Jobs         []string
	CACertsDir   string
	logger       *slog.Logger
}

func NewIngestJenkinsApp(logger *slog.Logger) *IngestJenkinsApp {
	return &IngestJenkinsApp{
		NATSHost: nats.DefaultHost,
		NATSPort: nats.DefaultPort,
		Jobs:     []string{},
		logger:   logger.With("service", "ingest_jenkins"),
	}
}

func (a *IngestJenkinsApp) Run(ctx context.Context) error {
	natsURL := nats.NATSConnectionString(a.NATSHost, a.NATSPort)
	publisher, publisherCloser, err := dalinats.NewPipelinePublisher(ctx, natsURL, a.logger.With("port", "domain", "connection", "nats"))
	if err != nil {
		return fmt.Errorf("create pipeline publisher: %w", err)
	}
	defer publisherCloser()

	httpCl, err := httpclient.NewHTTPClient(a.CACertsDir)
	if err != nil {
		return fmt.Errorf("building HTTP client: %w", err)
	}

	client := jenkins.NewClient(a.JenkinsURL, a.JenkinsUser, a.JenkinsToken, httpCl, a.logger)
	crawler := jenkins.NewCrawler(client, publisher, a.Jobs, a.logger)

	svc, err := jenkins.NewIngestJenkinsService(crawler, a.logger)
	if err != nil {
		return fmt.Errorf("creating ingest jenkins service: %w", err)
	}

	if err := svc.Run(ctx); err != nil {
		return fmt.Errorf("running ingest jenkins service: %w", err)
	}
	return nil
}
