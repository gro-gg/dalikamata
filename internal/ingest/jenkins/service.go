package jenkins

import (
	"context"
	"fmt"
	"log/slog"
)

type IngestJenkinsService struct {
	crawler *Crawler
	logger  *slog.Logger
}

func NewIngestJenkinsService(crawler *Crawler, logger *slog.Logger) (*IngestJenkinsService, error) {
	if crawler == nil {
		return nil, fmt.Errorf("invalid crawler, must not be nil")
	}
	return &IngestJenkinsService{
		crawler: crawler,
		logger:  logger.With("service", "ingest-jenkins"),
	}, nil
}

func (s *IngestJenkinsService) Run(ctx context.Context) error {
	s.logger.Info("Starting Ingest Jenkins Service")

	go func() {
		if err := s.crawler.Crawl(ctx); err != nil {
			s.logger.Error("crawling", "error", err)
		}
	}()

	<-ctx.Done()

	s.logger.Info("Stopping Ingest Jenkins Service")
	return nil
}
