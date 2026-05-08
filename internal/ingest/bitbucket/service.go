package bitbucket

import (
	"context"
	"fmt"
	"log/slog"

	"codeberg.org/aeforged/dalikamata/pkg/model"
)

// EventPublisher publishes ingest events to a message broker.
type EventPublisher interface {
	PublishCommit(context.Context, model.Commit) error
	PublishPullRequest(context.Context, model.PullRequest) error
	PublishRepo(context.Context, model.Repo) error
}

type IngestBitbucketService struct {
	crawler *Crawler
	logger  *slog.Logger
}

func NewIngestBitbucketService(crawler *Crawler, logger *slog.Logger) (*IngestBitbucketService, error) {
	l := logger.With("service", "inget-bitbucket")
	if crawler == nil {
		return nil, fmt.Errorf("invalid crawler, must not be nil")
	}

	s := &IngestBitbucketService{
		crawler: crawler,
		logger:  l,
	}

	return s, nil
}

func (s *IngestBitbucketService) Run(ctx context.Context) error {
	s.logger.Info("Starting Ingest Bitbucket Service")

	go func() {
		if err := s.crawler.Crawl(ctx); err != nil {
			s.logger.Error("crawling", "error", err)
		}
	}()

	s.logger.Info("Nothing left to do")
	<-ctx.Done()

	s.logger.Info("Stopping Ingest Bitbucket Service")
	return nil
}
