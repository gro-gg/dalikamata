package jenkins

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"
)

type IngestJenkinsService struct {
	crawler  *Crawler
	interval time.Duration
	logger   *slog.Logger
}

func NewIngestJenkinsService(crawler *Crawler, interval time.Duration, logger *slog.Logger) (*IngestJenkinsService, error) {
	if crawler == nil {
		return nil, fmt.Errorf("invalid crawler, must not be nil")
	}
	return &IngestJenkinsService{
		crawler:  crawler,
		interval: interval,
		logger:   logger.With("service", "ingest-jenkins"),
	}, nil
}

func (s *IngestJenkinsService) Run(ctx context.Context) error {
	s.logger.Info("Starting Ingest Jenkins Service", "interval", s.interval)
	s.runRefreshLoop(ctx)
	s.logger.Info("Stopping Ingest Jenkins Service")
	return nil
}

// runRefreshLoop crawls immediately, then repeats on every interval tick until
// ctx is cancelled. Concurrent ticks are skipped (with a warning) if the
// previous crawl is still running.
func (s *IngestJenkinsService) runRefreshLoop(ctx context.Context) {
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
