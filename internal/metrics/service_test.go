package metrics_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	"github.com/matryer/is"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/internal/domain/repo"
	"codeberg.org/aeforged/dalikamata/internal/metrics"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// stubAggregator implements metrics.QueryAggregator using a real
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
	must(t, svc.Refresh(context.Background()))

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
	must(t, svc.Refresh(context.Background()))

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

// newWorkflowFixtureAggregator builds a MemoryRepository seeded with two teams,
// two components, two workflows, and a mix of runs and tasks so we can assert
// the new metrics.
func newWorkflowFixtureAggregator(t *testing.T) *stubAggregator {
	t.Helper()
	r := repo.NewMemory()
	ctx := context.Background()

	mustAdd := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	// --- Team Alpha / component svc-a / workflow wf-build ---
	mustAdd(r.AddTeam(ctx, model.Team{Name: "alpha"}))
	mustAdd(r.AddComponent(ctx, model.Component{
		Name:     "svc-a",
		TeamName: "alpha",
		RepoIDs:  []string{"r1"},
	}))
	mustAdd(r.AddWorkflow(ctx, model.Workflow{ID: "wf-build", Name: "Build", RepoIDs: []string{"r1"}}))

	// Two runs: 90s success, 600s success.
	mustAdd(r.AddWorkflowRun(ctx, model.WorkflowRun{ID: "run1", WorkflowID: "wf-build", Status: "SUCCESS", Branch: "main", Duration: 90}))
	mustAdd(r.AddWorkflowRun(ctx, model.WorkflowRun{ID: "run2", WorkflowID: "wf-build", Status: "SUCCESS", Branch: "main", Duration: 600}))

	// Tasks for run1.
	mustAdd(r.AddWorkflowTask(ctx, model.WorkflowTask{WorkflowRunID: "run1", Order: 0, Name: "lint", Status: "SUCCESS", Duration: 30}))
	mustAdd(r.AddWorkflowTask(ctx, model.WorkflowTask{WorkflowRunID: "run1", Order: 1, Name: "test", Status: "SUCCESS", Duration: 60}))

	// Tasks for run2.
	mustAdd(r.AddWorkflowTask(ctx, model.WorkflowTask{WorkflowRunID: "run2", Order: 0, Name: "lint", Status: "SUCCESS", Duration: 28}))
	mustAdd(r.AddWorkflowTask(ctx, model.WorkflowTask{WorkflowRunID: "run2", Order: 1, Name: "test", Status: "FAILURE", Duration: 570}))

	// --- Team Beta / component svc-b / workflow wf-deploy (no component file → "unknown") ---
	mustAdd(r.AddWorkflow(ctx, model.Workflow{ID: "wf-orphan", Name: "Orphan Deploy"}))
	mustAdd(r.AddWorkflowRun(ctx, model.WorkflowRun{ID: "run3", WorkflowID: "wf-orphan", Status: "FAILURE", Duration: 45}))
	mustAdd(r.AddWorkflowTask(ctx, model.WorkflowTask{WorkflowRunID: "run3", Order: 0, Name: "deploy", Status: "FAILURE", Duration: 45}))

	return &stubAggregator{r: r}
}

// TestCollect_WorkflowRunDurationHistogram verifies that workflow_run_duration_seconds
// is emitted with the correct label values and counts.
func TestCollect_WorkflowRunDurationHistogram(t *testing.T) {
	is := is.New(t)
	agg := newWorkflowFixtureAggregator(t)
	svc := metrics.NewMetricsService(agg, discardLogger(), "")
	reg := prometheus.NewRegistry()
	reg.MustRegister(svc)
	must(t, svc.Refresh(context.Background()))

	families, err := reg.Gather()
	is.NoErr(err)

	// Find workflow_run_duration_seconds families.
	var wfFam *dto.MetricFamily
	for _, f := range families {
		if f.GetName() == "workflow_run_duration_seconds" {
			wfFam = f
			break
		}
	}
	is.True(wfFam != nil) // metric family must be present

	// There must be exactly 2 series: alpha/svc-a/wf-build/Build/SUCCESS and unknown/unknown/wf-orphan/Orphan Deploy/FAILURE.
	is.Equal(len(wfFam.GetMetric()), 2)

	// Verify the alpha series: 2 observations (90s, 600s).
	for _, m := range wfFam.GetMetric() {
		labels := labelMap(m)
		if labels["team_name"] == "alpha" {
			is.Equal(labels["component_name"], "svc-a")
			is.Equal(labels["workflow_name"], "Build")
			is.Equal(labels["branch"], "main")
			is.Equal(labels["status"], "SUCCESS")
			is.Equal(m.GetHistogram().GetSampleCount(), uint64(2))
			is.Equal(m.GetHistogram().GetSampleSum(), float64(90+600))
		}
	}
}

// TestCollect_WorkflowTaskDurationHistogram verifies that workflow_task_duration_seconds
// is emitted per task name, order, and status.
func TestCollect_WorkflowTaskDurationHistogram(t *testing.T) {
	is := is.New(t)
	agg := newWorkflowFixtureAggregator(t)
	svc := metrics.NewMetricsService(agg, discardLogger(), "")
	reg := prometheus.NewRegistry()
	reg.MustRegister(svc)
	must(t, svc.Refresh(context.Background()))

	families, err := reg.Gather()
	is.NoErr(err)

	var taskFam *dto.MetricFamily
	for _, f := range families {
		if f.GetName() == "workflow_task_duration_seconds" {
			taskFam = f
			break
		}
	}
	is.True(taskFam != nil)

	// Collect label combos for owned tasks (team=alpha), keyed by task+order+status.
	// Runs sharing the same (task, order, status) are merged into one series.
	type key struct{ task, order, status string }
	owned := map[key]bool{}
	for _, m := range taskFam.GetMetric() {
		labels := labelMap(m)
		if labels["team_name"] == "alpha" {
			owned[key{labels["task_name"], labels["task_order"], labels["status"]}] = true
		}
	}
	is.True(owned[key{"lint", "00", "SUCCESS"}])
	is.True(owned[key{"test", "01", "SUCCESS"}])
	is.True(owned[key{"test", "01", "FAILURE"}])
}

// TestCollect_WorkflowTaskOrderAfterJSONRoundtrip verifies that task_order
// labels survive a JSON marshal/unmarshal cycle, which is what happens when
// the aggregation result travels over NATS. encoding/json decodes all JSON
// numbers into float64 when the target type is any; intKey must handle that.
func TestCollect_WorkflowTaskOrderAfterJSONRoundtrip(t *testing.T) {
	is := is.New(t)

	// jsonRoundtripAggregator wraps a MemoryRepository but marshals and
	// unmarshals the aggregation result through JSON before returning it,
	// simulating the NATS transport layer.
	agg := &jsonRoundtripAggregator{r: newWorkflowFixtureAggregator(t).r}
	svc := metrics.NewMetricsService(agg, discardLogger(), "")
	reg := prometheus.NewRegistry()
	reg.MustRegister(svc)
	must(t, svc.Refresh(context.Background()))

	families, err := reg.Gather()
	is.NoErr(err)

	var taskFam *dto.MetricFamily
	for _, f := range families {
		if f.GetName() == "workflow_task_duration_seconds" {
			taskFam = f
			break
		}
	}
	is.True(taskFam != nil) // metric must be emitted even after JSON round-trip

	type key struct{ task, order, status string }
	owned := map[key]bool{}
	for _, m := range taskFam.GetMetric() {
		labels := labelMap(m)
		if labels["team_name"] == "alpha" {
			owned[key{labels["task_name"], labels["task_order"], labels["status"]}] = true
		}
	}
	is.True(owned[key{"lint", "00", "SUCCESS"}])
	is.True(owned[key{"test", "01", "SUCCESS"}])
}

type jsonRoundtripAggregator struct {
	r *repo.MemoryRepository
}

func (a *jsonRoundtripAggregator) Aggregate(ctx context.Context, q query.Query) (map[string]query.AggregationResult, error) {
	result, err := a.r.Aggregate(ctx, q)
	if err != nil || result == nil {
		return result, err
	}
	// Simulate NATS transport: marshal to JSON then unmarshal back.
	// This converts integer bucket keys (e.g. task_order) to float64.
	b, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	var out map[string]query.AggregationResult
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// labelMap extracts prometheus label pairs into a plain map for easy assertion.
func labelMap(m *dto.Metric) map[string]string {
	out := make(map[string]string, len(m.GetLabel()))
	for _, lp := range m.GetLabel() {
		out[lp.GetName()] = lp.GetValue()
	}
	return out
}
