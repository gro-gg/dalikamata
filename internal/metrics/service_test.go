package metrics_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/internal/domain/repo"
	"codeberg.org/aeforged/dalikamata/internal/metrics"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// stubAggregator implements metrics.PullRequestAggregator using a real
// MemoryRepository so we exercise the full aggregation path.
type stubAggregator struct {
	r *repo.MemoryRepository
}

func (s *stubAggregator) Aggregate(ctx context.Context, q query.Query) (map[string]query.AggregationResult, error) {
	return s.r.Aggregate(ctx, q)
}

func newFixtureAggregator(t *testing.T) *stubAggregator {
	t.Helper()
	fixedNow := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	r := repo.NewMemory(repo.WithClock(func() time.Time { return fixedNow }))
	ctx := context.Background()

	jan := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	janMerged := time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC) // 7 days = 604800s

	// One MERGED PR from January in repo "R".
	must(t, r.AddPullRequest(ctx, model.PullRequest{
		ID:        "1",
		RepoID:    "R",
		State:     model.PullRequestStateMerged,
		CreatedAt: jan,
		UpdatedAt: janMerged,
	}))
	return &stubAggregator{r: r}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// TestCollect_EmitsPRCycleTimeHistogram verifies that Collect produces a
// pr_cycle_time_seconds histogram with the correct label values and counts.
func TestCollect_EmitsPRCycleTimeHistogram(t *testing.T) {
	is := is.New(t)

	agg := newFixtureAggregator(t)
	svc := metrics.NewMetricsService(agg, discardLogger(), "")

	reg := prometheus.NewRegistry()
	reg.MustRegister(svc)

	// The MERGED PR has a cycle time of 7 days = 604800s.
	// It falls into the last bucket (≤ 604800).
	expected := strings.TrimSpace(`
# HELP pr_cycle_time_seconds Time from PR creation to current or final state in seconds.
# TYPE pr_cycle_time_seconds histogram
pr_cycle_time_seconds_bucket{created_month="2024-01",repo_id="R",state="MERGED",le="3600"} 0
pr_cycle_time_seconds_bucket{created_month="2024-01",repo_id="R",state="MERGED",le="14400"} 0
pr_cycle_time_seconds_bucket{created_month="2024-01",repo_id="R",state="MERGED",le="86400"} 0
pr_cycle_time_seconds_bucket{created_month="2024-01",repo_id="R",state="MERGED",le="259200"} 0
pr_cycle_time_seconds_bucket{created_month="2024-01",repo_id="R",state="MERGED",le="604800"} 1
pr_cycle_time_seconds_bucket{created_month="2024-01",repo_id="R",state="MERGED",le="+Inf"} 1
pr_cycle_time_seconds_sum{created_month="2024-01",repo_id="R",state="MERGED"} 604800
pr_cycle_time_seconds_count{created_month="2024-01",repo_id="R",state="MERGED"} 1
`) + "\n"

	err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "pr_cycle_time_seconds")
	is.NoErr(err)
}

// TestCollect_OpenPRUsesClockTime verifies that OPEN PRs use the pinned clock,
// not updatedAt.
func TestCollect_OpenPRUsesClockTime(t *testing.T) {
	is := is.New(t)

	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	fixedNow := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC) // 1.5 days = 129600s

	r := repo.NewMemory(repo.WithClock(func() time.Time { return fixedNow }))
	ctx := context.Background()
	must(t, r.AddPullRequest(ctx, model.PullRequest{
		ID:        "1",
		RepoID:    "R",
		State:     model.PullRequestStateOpen,
		CreatedAt: created,
		UpdatedAt: created,
	}))

	svc := metrics.NewMetricsService(&stubAggregator{r: r}, discardLogger(), "")
	reg := prometheus.NewRegistry()
	reg.MustRegister(svc)

	families, err := reg.Gather()
	is.NoErr(err)
	is.Equal(len(families), 1)
	m := families[0].GetMetric()
	is.Equal(len(m), 1)
	is.Equal(m[0].GetHistogram().GetSampleSum(), float64(fixedNow.Sub(created).Seconds()))
}

// TestCollect_EmptyRepo produces no metrics.
func TestCollect_EmptyRepo(t *testing.T) {
	is := is.New(t)
	r := repo.NewMemory()
	svc := metrics.NewMetricsService(&stubAggregator{r: r}, discardLogger(), "")
	reg := prometheus.NewRegistry()
	reg.MustRegister(svc)

	families, err := reg.Gather()
	is.NoErr(err)
	is.Equal(len(families), 0)
}
