package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"codeberg.org/aeforged/dalikamata/pkg/model"
)

const DefaultMetricsAddr = ":2112"

// PullRequestSubscriber is the port for receiving pull request events.
type PullRequestSubscriber interface {
	Subscribe(ctx context.Context, handler func(model.PullRequest)) error
}

type MetricsService struct {
	subscriber  PullRequestSubscriber
	logger      *slog.Logger
	registry    *prometheus.Registry
	prCycleTime *prometheus.HistogramVec
	metricsAddr string
}

func NewMetricsService(subscriber PullRequestSubscriber, logger *slog.Logger, metricsAddr string) *MetricsService {
	registry := prometheus.NewRegistry()

	prCycleTime := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pr_cycle_time_seconds",
			Help:    "Time from PR creation to current or final state in seconds.",
			Buckets: []float64{3600, 14400, 86400, 259200, 604800},
		},
		[]string{"repo_id", "created_month", "state"},
	)
	registry.MustRegister(prCycleTime)

	s := &MetricsService{
		subscriber:  subscriber,
		logger:      logger,
		registry:    registry,
		prCycleTime: prCycleTime,
		metricsAddr: DefaultMetricsAddr,
	}

	if metricsAddr != "" {
		s.metricsAddr = metricsAddr
	}

	return s
}

func (s *MetricsService) Run(ctx context.Context) (func(), error) {
	if err := s.subscriber.Subscribe(ctx, s.handlePullRequest); err != nil {
		return nil, fmt.Errorf("subscribing to pull requests: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{}))

	srv := &http.Server{
		Addr:    s.metricsAddr,
		Handler: mux,
	}

	go func() {
		s.logger.Info("Starting metrics HTTP server", "addr", s.metricsAddr)
		err := srv.ListenAndServe()
		if err != nil {
			s.logger.Error("starting metrics HTTP server", "error", err)
		}
	}()

	return func() {
		s.logger.Info("Shutting down metrics HTTP server")
		err := srv.Shutdown(ctx)
		if err != nil {
			s.logger.Error("shutting down metrics HTTP server", "error", err)
		}
	}, nil
}

func (s *MetricsService) handlePullRequest(pr model.PullRequest) {
	s.logger.Debug("received pull request event", "repo_id", pr.RepoID, "state", pr.State, "created_at", pr.CreatedAt)
	var cycleTime time.Duration
	switch pr.State {
	case "MERGED", "DECLINED":
		cycleTime = pr.UpdatedAt.Sub(pr.CreatedAt)
	default: // OPEN
		cycleTime = time.Since(pr.CreatedAt)
	}

	s.prCycleTime.WithLabelValues(
		pr.RepoID,
		pr.CreatedAt.Format("2006-01"),
		pr.State,
	).Observe(cycleTime.Seconds())

	s.logger.Info("observed pr cycle time", "repo_id", pr.RepoID, "state", pr.State, "seconds", cycleTime.Seconds())
}
