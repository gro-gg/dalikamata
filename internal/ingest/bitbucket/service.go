package bitbucket

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"
)

type IngestBitbucketService struct {
	crawler  *Crawler
	interval time.Duration
	logger   *slog.Logger
}

func NewIngestBitbucketService(crawler *Crawler, interval time.Duration, logger *slog.Logger) (*IngestBitbucketService, error) {
	l := logger.With("service", "ingest-bitbucket")
	if crawler == nil {
		return nil, fmt.Errorf("invalid crawler, must not be nil")
	}

	s := &IngestBitbucketService{
		crawler:  crawler,
		interval: interval,
		logger:   l,
	}

	return s, nil
}

func (s *IngestBitbucketService) Run(ctx context.Context) error {
	s.logger.Info("Starting Ingest Bitbucket Service", "interval", s.interval)
	s.runRefreshLoop(ctx)
	s.logger.Info("Stopping Ingest Bitbucket Service")
	return nil
}

// runRefreshLoop crawls immediately, then repeats on every interval tick until
// ctx is cancelled. Concurrent ticks are skipped (with a warning) if the
// previous crawl is still running.
func (s *IngestBitbucketService) runRefreshLoop(ctx context.Context) {
	var running atomic.Bool

	tick := func() {
		if !running.CompareAndSwap(false, true) {
			s.logger.Warn("crawl still running; skipping tick")
			return
		}
		go func() {
			defer running.Store(false)
			start := time.Now()
			if err := s.crawler.Crawl(ctx); err != nil {
				s.logger.Error("crawl failed", "error", err)
			}
			if elapsed := time.Since(start); elapsed > s.interval {
				s.logger.Warn("crawl exceeded interval; ticks may have been skipped",
					"elapsed", elapsed, "interval", s.interval)
			}
		}()
	}

	tick() // immediate first run

	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tick()
		}
	}
}
