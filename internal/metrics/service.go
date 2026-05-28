package metrics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
)

const DefaultMetricsAddr = "0.0.0.0:2112"

// PullRequestAggregator is the incoming port for obtaining server-side PR
// aggregation results from the domain.
type PullRequestAggregator interface {
	Aggregate(ctx context.Context, q query.Query) (map[string]query.AggregationResult, error)
}

// MetricsService implements prometheus.Collector. On every Prometheus scrape
// it requests a pre-computed PR cycle-time histogram from the domain and emits
// one const histogram per (repo_id, created_month, state) label combination.
type MetricsService struct {
	aggregator  PullRequestAggregator
	logger      *slog.Logger
	metricsAddr string
	prCycleDesc *prometheus.Desc
}

// prCycleBuckets mirrors the bucket boundaries of the original event-driven
// implementation so existing dashboards continue to work.
var prCycleBuckets = []float64{3600, 14400, 86400, 259200, 604800}

// prCycleQuery is the aggregation query issued on every scrape.
// terms(repo_id) → date_histogram(created_at, month) → terms(state) → histogram(cycle_time_seconds)
var prCycleQuery = query.Query{
	Entity: query.EntityPullRequest,
	Size:   -1,
	Aggs: map[string]query.Aggregation{
		"by_repo": {
			Op:    query.AggTerms,
			Field: query.PRRepoID,
			Aggs: map[string]query.Aggregation{
				"by_month": {
					Op:       query.AggDateHistogram,
					Field:    query.PRCreatedAt,
					Interval: "month",
					Format:   "2006-01",
					Aggs: map[string]query.Aggregation{
						"by_state": {
							Op:    query.AggTerms,
							Field: query.PRState,
							Aggs: map[string]query.Aggregation{
								"cycle_time": {
									Op:      query.AggHistogram,
									Field:   query.PRCycleTimeSeconds,
									Buckets: prCycleBuckets,
								},
							},
						},
					},
				},
			},
		},
	},
}

func NewMetricsService(aggregator PullRequestAggregator, logger *slog.Logger, metricsAddr string) *MetricsService {
	s := &MetricsService{
		aggregator:  aggregator,
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

// Collect implements prometheus.Collector. On every Prometheus scrape it
// issues one aggregation request to the domain and walks the result tree to
// emit one const histogram per (repo_id, created_month, state) group.
func (s *MetricsService) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := s.aggregator.Aggregate(ctx, prCycleQuery)
	if err != nil {
		s.logger.Error("querying pr cycle time aggregation", "error", err)
		return
	}
	if result == nil {
		return
	}

	if err := s.emitFromTree(ch, result); err != nil {
		s.logger.Error("emitting pr cycle time metrics", "error", err)
	}
}

// emitFromTree walks the by_repo → by_month → by_state → cycle_time tree
// and emits one MustNewConstHistogram per leaf.
func (s *MetricsService) emitFromTree(ch chan<- prometheus.Metric, result map[string]query.AggregationResult) error {
	byRepo, ok := result["by_repo"]
	if !ok {
		return nil
	}
	for _, repoBucket := range byRepo.Buckets {
		repoID, ok := repoBucket.Key.(string)
		if !ok {
			return fmt.Errorf("by_repo bucket key is %T, want string", repoBucket.Key)
		}
		byMonth, ok := repoBucket.Aggs["by_month"]
		if !ok {
			continue
		}
		for _, monthBucket := range byMonth.Buckets {
			month, ok := monthBucket.Key.(string)
			if !ok {
				return fmt.Errorf("by_month bucket key is %T, want string", monthBucket.Key)
			}
			byState, ok := monthBucket.Aggs["by_state"]
			if !ok {
				continue
			}
			for _, stateBucket := range byState.Buckets {
				state, ok := stateBucket.Key.(string)
				if !ok {
					return fmt.Errorf("by_state bucket key is %T, want string", stateBucket.Key)
				}
				cycleAgg, ok := stateBucket.Aggs["cycle_time"]
				if !ok {
					continue
				}
				bucketMap := make(map[float64]uint64, len(cycleAgg.Buckets))
				for _, b := range cycleAgg.Buckets {
					upper, ok := b.Key.(float64)
					if !ok {
						return fmt.Errorf("cycle_time bucket key is %T, want float64", b.Key)
					}
					bucketMap[upper] = b.DocCount
				}
				ch <- prometheus.MustNewConstHistogram(
					s.prCycleDesc,
					cycleAgg.DocCount,
					cycleAgg.Sum,
					bucketMap,
					repoID,
					month,
					state,
				)
				s.logger.Info("emitted pr cycle time metric",
					"repo_id", repoID,
					"state", state,
					"count", cycleAgg.DocCount,
				)
			}
		}
	}
	return nil
}
