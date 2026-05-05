package bitbucket

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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

func NewIngestBitbucketService(logger *slog.Logger, crawler *Crawler) (*IngestBitbucketService, error) {
	if crawler == nil {
		return nil, fmt.Errorf("invalid crawler, must not be nil")
	}

	s := &IngestBitbucketService{
		crawler: crawler,
		logger:  logger,
	}

	return s, nil
}

func (s *IngestBitbucketService) Run(ctx context.Context) error {
	s.logger.Info("Starting Ingest Bitbucket Service")

	go func() {
		s.logger.Info("Start Crawling Bitbucket")
		startTime := time.Now()
		if err := s.crawler.Crawl(ctx); err != nil {
			s.logger.Error("crawling", "error", err)
		}
		duration := time.Since(startTime)
		s.logger.Info("Finished Crawling Bitbucket", "duration", duration)

	}()

	s.logger.Info("Nothing left to do")
	s.logger.Info("Starting Ingest Bitbucket Service")

	return nil
}
