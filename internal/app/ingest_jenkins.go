package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
	"codeberg.org/aeforged/dalikamata/internal/httpclient"
	"codeberg.org/aeforged/dalikamata/internal/ingest/jenkins"
	internalnats "codeberg.org/aeforged/dalikamata/internal/nats"
)

const (
	defaultJenkinsInterval  = 5 * time.Minute
	jenkinsCursorBucketName = "ingest-jenkins-cursors"
)

type IngestJenkinsApp struct {
	JenkinsURL   string
	JenkinsUser  string
	JenkinsToken string
	NATSHost     string
	NATSPort     int
	Jobs         []string
	CACertsDir   string
	Interval     time.Duration
	logger       *slog.Logger
}

func NewIngestJenkinsApp(logger *slog.Logger) *IngestJenkinsApp {
	return &IngestJenkinsApp{
		NATSHost: internalnats.DefaultHost,
		NATSPort: internalnats.DefaultPort,
		Jobs:     []string{},
		Interval: defaultJenkinsInterval,
		logger:   logger.With("service", "ingest_jenkins"),
	}
}

func (a *IngestJenkinsApp) Run(ctx context.Context) error {
	natsURL := internalnats.NATSConnectionString(a.NATSHost, a.NATSPort)

	publisher, publisherCloser, err := dalinats.NewPipelinePublisher(ctx, natsURL, a.logger.With("port", "domain", "connection", "nats"))
	if err != nil {
		return fmt.Errorf("create pipeline publisher: %w", err)
	}
	defer publisherCloser()

	_, js, jsCloser, err := internalnats.Connect(ctx, natsURL, a.logger.With("connection", "nats", "client", "cursor-store"))
	if err != nil {
		return fmt.Errorf("connecting to NATS for cursor store: %w", err)
	}
	defer jsCloser()

	cursors, err := jenkins.NewJetStreamCursors(ctx, js, jenkinsCursorBucketName)
	if err != nil {
		return fmt.Errorf("creating cursor store: %w", err)
	}

	httpCl, err := httpclient.NewHTTPClient(a.CACertsDir)
	if err != nil {
		return fmt.Errorf("building HTTP client: %w", err)
	}

	client := jenkins.NewClient(a.JenkinsURL, a.JenkinsUser, a.JenkinsToken, httpCl, a.logger)
	crawler := jenkins.NewCrawler(client, publisher, cursors, a.Jobs, a.logger)

	svc, err := jenkins.NewIngestJenkinsService(crawler, a.Interval, a.logger)
	if err != nil {
		return fmt.Errorf("creating ingest jenkins service: %w", err)
	}

	if err := svc.Run(ctx); err != nil {
		return fmt.Errorf("running ingest jenkins service: %w", err)
	}
	return nil
}
