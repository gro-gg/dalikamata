package bitbucket

import (
	"context"
	"fmt"
	"log/slog"
)

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
