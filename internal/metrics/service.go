package metrics

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

const DefaultMetricsAddr = "0.0.0.0:2112"

// PullRequestQuerier is the incoming port for fetching pull request state from
// the domain. At scrape time the MetricsService calls QueryPullRequestsAll to
// obtain all stored PRs and derive its metrics from them.
type PullRequestQuerier interface {
	QueryPullRequestsAll(ctx context.Context, q query.Query) ([]model.PullRequest, error)
}

// MetricsService implements prometheus.Collector. On every Prometheus scrape
// it queries the domain service for all pull requests and emits fresh
// cycle-time histogram observations. No event stream subscription is held.
type MetricsService struct {
	querier     PullRequestQuerier
	logger      *slog.Logger
	metricsAddr string
	prCycleDesc *prometheus.Desc
}

// prCycleBuckets mirrors the bucket boundaries of the original event-driven
// implementation so existing dashboards continue to work.
var prCycleBuckets = []float64{3600, 14400, 86400, 259200, 604800}

func NewMetricsService(querier PullRequestQuerier, logger *slog.Logger, metricsAddr string) *MetricsService {
	s := &MetricsService{
		querier:     querier,
		logger:      logger,
		metricsAddr: DefaultMetricsAddr,
		prCycleDesc: prometheus.NewDesc(
			"pr_cycle_time_seconds",
			"Time from PR creation to current or final state in seconds.",
			[]string{"repo_id", "created_month", "state"},
			nil,
		),
	}
	if metricsAddr != "" {
		s.metricsAddr = metricsAddr
	}
	return s
}

// Run registers the MetricsService as a Prometheus collector, starts the HTTP
// server, and blocks until ctx is cancelled.
func (s *MetricsService) Run(ctx context.Context) error {
	registry := prometheus.NewRegistry()
	registry.MustRegister(s)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	srv := &http.Server{
		Addr:    s.metricsAddr,
		Handler: mux,
	}

	go func() {
		s.logger.Info("starting metrics HTTP server", "addr", s.metricsAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("metrics HTTP server error", "error", err)
		}
	}()

	<-ctx.Done()

	if err := srv.Shutdown(context.Background()); err != nil {
		s.logger.Error("shutting down metrics HTTP server", "error", err)
	}
	return nil
}

// Describe implements prometheus.Collector.
func (s *MetricsService) Describe(ch chan<- *prometheus.Desc) {
	ch <- s.prCycleDesc
}

// Collect implements prometheus.Collector. It is called on every Prometheus
// scrape. It queries all pull requests from the domain, groups them by
// (repo_id, created_month, state), and emits one const histogram per group.
func (s *MetricsService) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prs, err := s.querier.QueryPullRequestsAll(ctx, query.Query{Entity: query.EntityPullRequest})
	if err != nil {
		s.logger.Error("querying pull requests for metrics collection", "error", err)
		return
	}
	s.logger.Debug("collected pull requests for metrics", "count", len(prs))

	type labelKey struct{ repoID, createdMonth, state string }
	type histData struct {
		count   uint64
		sum     float64
		buckets map[float64]uint64
	}

	groups := make(map[labelKey]*histData)

	for _, pr := range prs {
		var cycleTime time.Duration
		switch pr.State {
		case model.PullRequestStateMerged, model.PullRequestStateDeclined:
			cycleTime = pr.UpdatedAt.Sub(pr.CreatedAt)
		default: // OPEN — measure time so far
			cycleTime = time.Since(pr.CreatedAt)
		}
		secs := cycleTime.Seconds()

		k := labelKey{
			repoID:       pr.RepoID,
			createdMonth: pr.CreatedAt.Format("2006-01"),
			state:        pr.State,
		}
		g, ok := groups[k]
		if !ok {
			g = &histData{buckets: make(map[float64]uint64, len(prCycleBuckets))}
			for _, b := range prCycleBuckets {
				g.buckets[b] = 0
			}
			groups[k] = g
		}
		g.count++
		g.sum += secs
		// Prometheus histograms are cumulative: increment every bucket
		// whose upper bound is >= the observed value.
		for _, b := range prCycleBuckets {
			if secs <= b {
				g.buckets[b]++
			}
		}
	}

	for k, g := range groups {
		ch <- prometheus.MustNewConstHistogram(
			s.prCycleDesc,
			g.count,
			g.sum,
			g.buckets,
			k.repoID,
			k.createdMonth,
			k.state,
		)
		s.logger.Info("emitted pr cycle time metric",
			"repo_id", k.repoID,
			"state", k.state,
			"count", g.count,
		)
	}
}
