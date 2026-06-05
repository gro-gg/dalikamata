package metrics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
)

const DefaultMetricsAddr = "0.0.0.0:2112"

const (
	DefaultRefreshInterval  = 30 * time.Second
	DefaultAggregateTimeout = 30 * time.Second
)

// PullRequestAggregator is the incoming port for obtaining server-side PR
// aggregation results from the domain.
type PullRequestAggregator interface {
	Aggregate(ctx context.Context, q query.Query) (map[string]query.AggregationResult, error)
}

// Option configures a MetricsService.
type Option func(*MetricsService)

// WithRefreshInterval sets how often background loops re-compute each metric.
func WithRefreshInterval(d time.Duration) Option {
	return func(s *MetricsService) { s.refreshInterval = d }
}

// WithAggregateTimeout sets the per-Aggregate call deadline used by each loop iteration.
func WithAggregateTimeout(d time.Duration) Option {
	return func(s *MetricsService) { s.aggregateTimeout = d }
}

// cachedAggregation holds the last successful aggregation tree for one metric.
type cachedAggregation struct {
	tree    map[string]query.AggregationResult
	fetched time.Time
}

// MetricsService implements prometheus.Collector. Background goroutines
// periodically refresh each metric by querying the domain; Collect emits the
// last cached values so Prometheus scrapes never block on live queries.
type MetricsService struct {
	aggregator  PullRequestAggregator
	logger      *slog.Logger
	metricsAddr string
	prCycleDesc *prometheus.Desc
	wfRunDesc   *prometheus.Desc
	wfTaskDesc  *prometheus.Desc

	refreshInterval  time.Duration
	aggregateTimeout time.Duration

	prCycleCache atomic.Pointer[cachedAggregation]
	wfRunCache   atomic.Pointer[cachedAggregation]
	wfTaskCache  atomic.Pointer[cachedAggregation]
}

// prCycleBuckets mirrors the bucket boundaries of the original event-driven
// implementation so existing dashboards continue to work.
var prCycleBuckets = []float64{3600, 14400, 86400, 259200, 604800}

// workflowRunBuckets covers typical CI/CD pipeline total runtimes (1m–6h).
var workflowRunBuckets = []float64{60, 300, 900, 1800, 3600, 7200, 21600}

// workflowTaskBuckets covers individual stage durations (30s–1h).
var workflowTaskBuckets = []float64{30, 60, 120, 300, 600, 1800, 3600}

// workflowRunQuery aggregates run durations by team → component → workflow_id → workflow_name → status.
var workflowRunQuery = query.Query{
	Entity: query.EntityWorkflowRun,
	Size:   -1,
	Aggs: map[string]query.Aggregation{
		"by_team": {
			Op:    query.AggTerms,
			Field: query.RunTeamName,
			Aggs: map[string]query.Aggregation{
				"by_component": {
					Op:    query.AggTerms,
					Field: query.RunComponentName,
					Aggs: map[string]query.Aggregation{
						"by_workflow_id": {
							Op:    query.AggTerms,
							Field: query.RunWorkflowID,
							Aggs: map[string]query.Aggregation{
								"by_workflow_name": {
									Op:    query.AggTerms,
									Field: query.RunWorkflowName,
									Aggs: map[string]query.Aggregation{
										"by_status": {
											Op:    query.AggTerms,
											Field: query.RunStatus,
											Aggs: map[string]query.Aggregation{
												"duration": {
													Op:      query.AggHistogram,
													Field:   query.RunDuration,
													Buckets: workflowRunBuckets,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},
}

// workflowTaskQuery aggregates task durations by team → component → workflow_id → workflow_name → task_name → task_order → workflow_run_id → status.
var workflowTaskQuery = query.Query{
	Entity: query.EntityWorkflowTask,
	Size:   -1,
	Aggs: map[string]query.Aggregation{
		"by_team": {
			Op:    query.AggTerms,
			Field: query.TaskTeamName,
			Aggs: map[string]query.Aggregation{
				"by_component": {
					Op:    query.AggTerms,
					Field: query.TaskComponentName,
					Aggs: map[string]query.Aggregation{
						"by_workflow_id": {
							Op:    query.AggTerms,
							Field: query.TaskWorkflowID,
							Aggs: map[string]query.Aggregation{
								"by_workflow_name": {
									Op:    query.AggTerms,
									Field: query.TaskWorkflowName,
									Aggs: map[string]query.Aggregation{
										"by_task_name": {
											Op:    query.AggTerms,
											Field: query.TaskName,
											Aggs: map[string]query.Aggregation{
												"by_task_order": {
													Op:    query.AggTerms,
													Field: query.TaskOrder,
													Aggs: map[string]query.Aggregation{
														"by_run_id": {
															Op:    query.AggTerms,
															Field: query.TaskWorkflowRunID,
															Aggs: map[string]query.Aggregation{
																"by_status": {
																	Op:    query.AggTerms,
																	Field: query.TaskStatus,
																	Aggs: map[string]query.Aggregation{
																		"duration": {
																			Op:      query.AggHistogram,
																			Field:   query.TaskDuration,
																			Buckets: workflowTaskBuckets,
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},
}

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

func NewMetricsService(aggregator PullRequestAggregator, logger *slog.Logger, metricsAddr string, opts ...Option) *MetricsService {
	s := &MetricsService{
		aggregator:       aggregator,
		logger:           logger,
		metricsAddr:      DefaultMetricsAddr,
		refreshInterval:  DefaultRefreshInterval,
		aggregateTimeout: DefaultAggregateTimeout,
		prCycleDesc: prometheus.NewDesc(
			"pr_cycle_time_seconds",
			"Time from PR creation to current or final state in seconds.",
			[]string{"repo_id", "created_month", "state"},
			nil,
		),
		wfRunDesc: prometheus.NewDesc(
			"workflow_run_duration_seconds",
			"Total duration of a workflow run in seconds, grouped by team and component.",
			[]string{"team_name", "component_name", "workflow_id", "workflow_name", "status"},
			nil,
		),
		wfTaskDesc: prometheus.NewDesc(
			"workflow_task_duration_seconds",
			"Duration of a workflow stage/task in seconds, grouped by team and component.",
			[]string{"team_name", "component_name", "workflow_id", "workflow_name", "task_name", "task_order", "workflow_run_id", "status"},
			nil,
		),
	}
	if metricsAddr != "" {
		s.metricsAddr = metricsAddr
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Run registers the MetricsService as a Prometheus collector, starts the HTTP
// server, launches one background refresh loop per metric, and blocks until
// ctx is cancelled. Prometheus scrapes are served from cached data and never
// block on live aggregation queries.
func (s *MetricsService) Run(ctx context.Context) error {
	registry := prometheus.NewRegistry()
	registry.MustRegister(s)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	srv := &http.Server{
		Addr:    s.metricsAddr,
		Handler: mux,
	}

	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		s.logger.Info("starting metrics HTTP server", "addr", s.metricsAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("metrics HTTP server error", "error", err)
		}
	}()
	go func() { defer wg.Done(); s.runMetricLoop(ctx, "pr_cycle_time", s.refreshPRCycle) }()
	go func() { defer wg.Done(); s.runMetricLoop(ctx, "workflow_run_duration", s.refreshWorkflowRun) }()
	go func() { defer wg.Done(); s.runMetricLoop(ctx, "workflow_task_duration", s.refreshWorkflowTask) }()

	<-ctx.Done()

	if err := srv.Shutdown(context.Background()); err != nil {
		s.logger.Error("shutting down metrics HTTP server", "error", err)
	}
	wg.Wait()
	return nil
}

// Describe implements prometheus.Collector.
func (s *MetricsService) Describe(ch chan<- *prometheus.Desc) {
	ch <- s.prCycleDesc
	ch <- s.wfRunDesc
	ch <- s.wfTaskDesc
}

// Collect implements prometheus.Collector. It emits the last cached histogram
// values for each metric. If a metric has not been computed yet, it is omitted.
func (s *MetricsService) Collect(ch chan<- prometheus.Metric) {
	if c := s.prCycleCache.Load(); c != nil {
		if err := s.emitPRCycleTree(ch, c.tree); err != nil {
			s.logger.Error("emitting pr cycle time metrics", "error", err)
		}
	}
	if c := s.wfRunCache.Load(); c != nil {
		if err := s.emitWorkflowRunTree(ch, c.tree); err != nil {
			s.logger.Error("emitting workflow run duration metrics", "error", err)
		}
	}
	if c := s.wfTaskCache.Load(); c != nil {
		if err := s.emitWorkflowTaskTree(ch, c.tree); err != nil {
			s.logger.Error("emitting workflow task duration metrics", "error", err)
		}
	}
}

// Refresh synchronously computes all three metrics and stores them in the
// cache. Tests call this to populate the cache before Gather().
func (s *MetricsService) Refresh(ctx context.Context) error {
	var errs []error
	if err := s.refreshPRCycle(ctx); err != nil {
		errs = append(errs, fmt.Errorf("pr_cycle: %w", err))
	}
	if err := s.refreshWorkflowRun(ctx); err != nil {
		errs = append(errs, fmt.Errorf("workflow_run: %w", err))
	}
	if err := s.refreshWorkflowTask(ctx); err != nil {
		errs = append(errs, fmt.Errorf("workflow_task: %w", err))
	}
	return errors.Join(errs...)
}

func (s *MetricsService) refreshPRCycle(ctx context.Context) error {
	res, err := s.aggregator.Aggregate(ctx, prCycleQuery)
	if err != nil {
		return err
	}
	s.prCycleCache.Store(&cachedAggregation{tree: res, fetched: time.Now()})
	return nil
}

func (s *MetricsService) refreshWorkflowRun(ctx context.Context) error {
	res, err := s.aggregator.Aggregate(ctx, workflowRunQuery)
	if err != nil {
		return err
	}
	s.wfRunCache.Store(&cachedAggregation{tree: res, fetched: time.Now()})
	return nil
}

func (s *MetricsService) refreshWorkflowTask(ctx context.Context) error {
	res, err := s.aggregator.Aggregate(ctx, workflowTaskQuery)
	if err != nil {
		return err
	}
	s.wfTaskCache.Store(&cachedAggregation{tree: res, fetched: time.Now()})
	return nil
}

// runMetricLoop runs refresh immediately then on each tick of s.refreshInterval
// until ctx is cancelled. Errors are logged; a failed refresh retains the last
// cached value so dashboards see stale data rather than disappearing series.
func (s *MetricsService) runMetricLoop(ctx context.Context, name string, refresh func(context.Context) error) {
	s.tickRefresh(ctx, name, refresh)
	t := time.NewTicker(s.refreshInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tickRefresh(ctx, name, refresh)
		}
	}
}

func (s *MetricsService) tickRefresh(ctx context.Context, name string, refresh func(context.Context) error) {
	start := time.Now()
	rctx, cancel := context.WithTimeout(ctx, s.aggregateTimeout)
	defer cancel()
	if err := refresh(rctx); err != nil && !errors.Is(err, context.Canceled) {
		s.logger.Error("metric refresh failed", "metric", name, "error", err)
	}
	if elapsed := time.Since(start); elapsed > s.refreshInterval {
		s.logger.Warn("metric refresh exceeded interval; ticks may have been dropped",
			"metric", name, "elapsed", elapsed, "interval", s.refreshInterval)
	}
}

// emitPRCycleTree walks the by_repo → by_month → by_state → cycle_time tree
// and emits one MustNewConstHistogram per leaf.
func (s *MetricsService) emitPRCycleTree(ch chan<- prometheus.Metric, result map[string]query.AggregationResult) error {
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

// extractBucketMap converts a histogram AggregationResult's Buckets into the
// cumulative map[upperBound]count that MustNewConstHistogram expects.
func extractBucketMap(agg query.AggregationResult) (map[float64]uint64, error) {
	m := make(map[float64]uint64, len(agg.Buckets))
	for _, b := range agg.Buckets {
		upper, ok := b.Key.(float64)
		if !ok {
			return nil, fmt.Errorf("histogram bucket key is %T, want float64", b.Key)
		}
		m[upper] = b.DocCount
	}
	return m, nil
}

// strKey asserts that a bucket key is a string.
func strKey(key any, label string) (string, error) {
	s, ok := key.(string)
	if !ok {
		return "", fmt.Errorf("%s bucket key is %T, want string", label, key)
	}
	return s, nil
}

// intKey asserts that a bucket key is an integer.
// float64 is included because encoding/json unmarshals all JSON numbers into
// float64 when the target type is any, which applies after NATS round-trips.
func intKey(key any, label string) (int, error) {
	switch v := key.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("%s bucket key is %T, want int", label, key)
	}
}

// emitWorkflowRunTree walks team→component→workflow_id→workflow_name→status→duration
// and emits one workflow_run_duration_seconds histogram per leaf.
func (s *MetricsService) emitWorkflowRunTree(ch chan<- prometheus.Metric, result map[string]query.AggregationResult) error {
	byTeam, ok := result["by_team"]
	if !ok {
		return nil
	}
	for _, teamB := range byTeam.Buckets {
		team, err := strKey(teamB.Key, "by_team")
		if err != nil {
			return err
		}
		for _, compB := range teamB.Aggs["by_component"].Buckets {
			comp, err := strKey(compB.Key, "by_component")
			if err != nil {
				return err
			}
			for _, wfIDB := range compB.Aggs["by_workflow_id"].Buckets {
				wfID, err := strKey(wfIDB.Key, "by_workflow_id")
				if err != nil {
					return err
				}
				for _, wfNameB := range wfIDB.Aggs["by_workflow_name"].Buckets {
					wfName, err := strKey(wfNameB.Key, "by_workflow_name")
					if err != nil {
						return err
					}
					for _, statusB := range wfNameB.Aggs["by_status"].Buckets {
						status, err := strKey(statusB.Key, "by_status")
						if err != nil {
							return err
						}
						durAgg, ok := statusB.Aggs["duration"]
						if !ok {
							continue
						}
						bm, err := extractBucketMap(durAgg)
						if err != nil {
							return err
						}
						ch <- prometheus.MustNewConstHistogram(
							s.wfRunDesc,
							durAgg.DocCount,
							durAgg.Sum,
							bm,
							team, comp, wfID, wfName, status,
						)
					}
				}
			}
		}
	}
	return nil
}

// emitWorkflowTaskTree walks team→component→workflow_id→workflow_name→task_name→task_order→workflow_run_id→status→duration
// and emits one workflow_task_duration_seconds histogram per leaf.
func (s *MetricsService) emitWorkflowTaskTree(ch chan<- prometheus.Metric, result map[string]query.AggregationResult) error {
	byTeam, ok := result["by_team"]
	if !ok {
		return nil
	}
	for _, teamB := range byTeam.Buckets {
		team, err := strKey(teamB.Key, "by_team")
		if err != nil {
			return err
		}
		for _, compB := range teamB.Aggs["by_component"].Buckets {
			comp, err := strKey(compB.Key, "by_component")
			if err != nil {
				return err
			}
			for _, wfIDB := range compB.Aggs["by_workflow_id"].Buckets {
				wfID, err := strKey(wfIDB.Key, "by_workflow_id")
				if err != nil {
					return err
				}
				for _, wfNameB := range wfIDB.Aggs["by_workflow_name"].Buckets {
					wfName, err := strKey(wfNameB.Key, "by_workflow_name")
					if err != nil {
						return err
					}
					for _, taskB := range wfNameB.Aggs["by_task_name"].Buckets {
						taskName, err := strKey(taskB.Key, "by_task_name")
						if err != nil {
							return err
						}
						for _, orderB := range taskB.Aggs["by_task_order"].Buckets {
							order, err := intKey(orderB.Key, "by_task_order")
							if err != nil {
								return err
							}
							taskOrder := fmt.Sprintf("%02d", order)
							for _, runB := range orderB.Aggs["by_run_id"].Buckets {
								runID, err := strKey(runB.Key, "by_run_id")
								if err != nil {
									return err
								}
								for _, statusB := range runB.Aggs["by_status"].Buckets {
									status, err := strKey(statusB.Key, "by_status")
									if err != nil {
										return err
									}
									durAgg, ok := statusB.Aggs["duration"]
									if !ok {
										continue
									}
									bm, err := extractBucketMap(durAgg)
									if err != nil {
										return err
									}
									ch <- prometheus.MustNewConstHistogram(
										s.wfTaskDesc,
										durAgg.DocCount,
										durAgg.Sum,
										bm,
										team, comp, wfID, wfName, taskName, taskOrder, runID, status,
									)
								}
							}
						}
					}
				}
			}
		}
	}
	return nil
}
