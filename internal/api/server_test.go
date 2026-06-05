package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/pkg/model"
)

// fakeQueryFetcher records the last query passed to each method and returns
// pre-configured responses for use in table-driven tests.
type fakeQueryFetcher struct {
	lastQuery    query.Query
	workflowRuns []model.WorkflowRun
	workflowTasks []model.WorkflowTask
	aggs         map[string]query.AggregationResult
	err          error
}

func (f *fakeQueryFetcher) QueryReposAll(ctx context.Context, q query.Query) ([]model.Repo, error) {
	f.lastQuery = q; return nil, f.err
}
func (f *fakeQueryFetcher) QueryCommitsAll(ctx context.Context, q query.Query) ([]model.Commit, error) {
	f.lastQuery = q; return nil, f.err
}
func (f *fakeQueryFetcher) QueryPullRequestsAll(ctx context.Context, q query.Query) ([]model.PullRequest, error) {
	f.lastQuery = q; return nil, f.err
}
func (f *fakeQueryFetcher) QueryWorkflowsAll(ctx context.Context, q query.Query) ([]model.Workflow, error) {
	f.lastQuery = q; return nil, f.err
}
func (f *fakeQueryFetcher) QueryWorkflowRunsAll(ctx context.Context, q query.Query) ([]model.WorkflowRun, error) {
	f.lastQuery = q; return f.workflowRuns, f.err
}
func (f *fakeQueryFetcher) QueryWorkflowTasksAll(ctx context.Context, q query.Query) ([]model.WorkflowTask, error) {
	f.lastQuery = q; return f.workflowTasks, f.err
}
func (f *fakeQueryFetcher) QueryTeamsAll(ctx context.Context, q query.Query) ([]model.Team, error) {
	f.lastQuery = q; return nil, f.err
}
func (f *fakeQueryFetcher) QueryComponentsAll(ctx context.Context, q query.Query) ([]model.Component, error) {
	f.lastQuery = q; return nil, f.err
}
func (f *fakeQueryFetcher) Aggregate(ctx context.Context, q query.Query) (map[string]query.AggregationResult, error) {
	f.lastQuery = q; return f.aggs, f.err
}

// --- parseQueryParams unit tests ---

func TestParseQueryParams_Defaults(t *testing.T) {
	q, err := parseQueryParams(url.Values{}, query.EntityWorkflowRun)
	if err != nil {
		t.Fatal(err)
	}
	if q.Entity != query.EntityWorkflowRun {
		t.Errorf("entity = %v, want %v", q.Entity, query.EntityWorkflowRun)
	}
	if q.Size != defaultSize {
		t.Errorf("size = %v, want %v", q.Size, defaultSize)
	}
	if q.From != 0 {
		t.Errorf("from = %v, want 0", q.From)
	}
	if q.Filter != nil {
		t.Errorf("filter = %v, want nil", q.Filter)
	}
}

func TestParseQueryParams_SizeFrom(t *testing.T) {
	q, err := parseQueryParams(url.Values{"size": {"10"}, "from": {"20"}}, query.EntityWorkflowRun)
	if err != nil {
		t.Fatal(err)
	}
	if q.Size != 10 {
		t.Errorf("size = %v, want 10", q.Size)
	}
	if q.From != 20 {
		t.Errorf("from = %v, want 20", q.From)
	}
}

func TestParseQueryParams_Sort(t *testing.T) {
	q, err := parseQueryParams(url.Values{"sort": {"-started_at,workflow_run_id"}}, query.EntityWorkflowTask)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Sort) != 2 {
		t.Fatalf("sort len = %v, want 2", len(q.Sort))
	}
	if q.Sort[0].Field != "started_at" || q.Sort[0].Order != query.SortDesc {
		t.Errorf("sort[0] = %+v", q.Sort[0])
	}
	if q.Sort[1].Field != "workflow_run_id" || q.Sort[1].Order != query.SortAsc {
		t.Errorf("sort[1] = %+v", q.Sort[1])
	}
}

func TestParseQueryParams_TermFilter(t *testing.T) {
	q, err := parseQueryParams(url.Values{"filter.team_name": {"platform"}}, query.EntityWorkflowRun)
	if err != nil {
		t.Fatal(err)
	}
	if q.Filter == nil {
		t.Fatal("filter is nil")
	}
	if q.Filter.Op != query.OpTerm {
		t.Errorf("op = %v, want %v", q.Filter.Op, query.OpTerm)
	}
	if q.Filter.Field != "team_name" {
		t.Errorf("field = %v, want team_name", q.Filter.Field)
	}
	if q.Filter.Value.String != "platform" {
		t.Errorf("value = %v, want platform", q.Filter.Value.String)
	}
}

func TestParseQueryParams_TermsFilter(t *testing.T) {
	q, err := parseQueryParams(url.Values{"filter.workflow_run_id": {"run-a", "run-b"}}, query.EntityWorkflowTask)
	if err != nil {
		t.Fatal(err)
	}
	if q.Filter == nil {
		t.Fatal("filter is nil")
	}
	if q.Filter.Op != query.OpTerms {
		t.Errorf("op = %v, want %v", q.Filter.Op, query.OpTerms)
	}
	if len(q.Filter.Values) != 2 {
		t.Errorf("values len = %v, want 2", len(q.Filter.Values))
	}
}

func TestParseQueryParams_RangeFilter(t *testing.T) {
	params := url.Values{
		"filter.started_at.gte": {"2026-01-01T00:00:00Z"},
		"filter.started_at.lte": {"2026-06-01T00:00:00Z"},
	}
	q, err := parseQueryParams(params, query.EntityWorkflowRun)
	if err != nil {
		t.Fatal(err)
	}
	if q.Filter == nil {
		t.Fatal("filter is nil")
	}
	if q.Filter.Op != query.OpRange {
		t.Errorf("op = %v, want %v", q.Filter.Op, query.OpRange)
	}
	if q.Filter.Range.GTE == nil || q.Filter.Range.LTE == nil {
		t.Errorf("expected both GTE and LTE to be set, got %+v", q.Filter.Range)
	}
	want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !q.Filter.Range.GTE.Time.Equal(want) {
		t.Errorf("GTE time = %v, want %v", q.Filter.Range.GTE.Time, want)
	}
}

func TestParseQueryParams_ExistsFilter(t *testing.T) {
	q, err := parseQueryParams(url.Values{"filter.commit_sha.exists": {"true"}}, query.EntityWorkflowRun)
	if err != nil {
		t.Fatal(err)
	}
	if q.Filter == nil {
		t.Fatal("filter is nil")
	}
	if q.Filter.Op != query.OpExists {
		t.Errorf("op = %v, want %v", q.Filter.Op, query.OpExists)
	}
	if q.Filter.Field != "commit_sha" {
		t.Errorf("field = %v, want commit_sha", q.Filter.Field)
	}
}

func TestParseQueryParams_MultipleFilters_BoolMust(t *testing.T) {
	params := url.Values{
		"filter.team_name":  {"platform"},
		"filter.status":     {"SUCCESS"},
	}
	q, err := parseQueryParams(params, query.EntityWorkflowRun)
	if err != nil {
		t.Fatal(err)
	}
	if q.Filter == nil {
		t.Fatal("filter is nil")
	}
	if q.Filter.Op != query.OpBool {
		t.Errorf("op = %v, want %v", q.Filter.Op, query.OpBool)
	}
	if len(q.Filter.Must) != 2 {
		t.Errorf("must len = %v, want 2", len(q.Filter.Must))
	}
}

func TestParseQueryParams_UnknownField_ReturnsError(t *testing.T) {
	_, err := parseQueryParams(url.Values{"filter.nonexistent_field": {"val"}}, query.EntityWorkflowRun)
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestParseQueryParams_InvalidSize_ReturnsError(t *testing.T) {
	_, err := parseQueryParams(url.Values{"size": {"notanumber"}}, query.EntityWorkflowRun)
	if err == nil {
		t.Fatal("expected error for invalid size, got nil")
	}
}

// --- HTTP handler tests ---

func newTestServer(f *fakeQueryFetcher) *Server {
	return NewServer(f, discardLogger(), "", WithQueryTimeout(5*time.Second))
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHandler_GET_WorkflowRuns_200(t *testing.T) {
	fake := &fakeQueryFetcher{
		workflowRuns: []model.WorkflowRun{{ID: "run-1", Status: "SUCCESS"}},
	}
	srv := newTestServer(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflowRuns?size=5&sort=-started_at", nil)
	w := httptest.NewRecorder()
	srv.newMux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %v, want 200", w.Code)
	}
	var resp hitsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Entity != "workflowRun" {
		t.Errorf("entity = %v, want workflowRun", resp.Entity)
	}
	if resp.Size != 5 {
		t.Errorf("size = %v, want 5", resp.Size)
	}
	if fake.lastQuery.Sort[0].Field != "started_at" || fake.lastQuery.Sort[0].Order != query.SortDesc {
		t.Errorf("sort not propagated: %+v", fake.lastQuery.Sort)
	}
}

func TestHandler_GET_UnknownField_400(t *testing.T) {
	srv := newTestServer(&fakeQueryFetcher{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflowRuns?filter.bogus_field=x", nil)
	w := httptest.NewRecorder()
	srv.newMux().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %v, want 400", w.Code)
	}
}

func TestHandler_POST_QueryBodyPassthrough(t *testing.T) {
	fake := &fakeQueryFetcher{}
	srv := newTestServer(fake)

	body := query.Query{
		Entity: query.EntityWorkflowRun,
		Filter: &query.Filter{
			Op:    query.OpTerm,
			Field: query.RunTeamName,
			Value: ptr(query.StringValue("alpha")),
		},
		Size: 3,
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflowRuns", bytes.NewReader(b))
	w := httptest.NewRecorder()
	srv.newMux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %v, want 200; body: %s", w.Code, w.Body.String())
	}
	if fake.lastQuery.Filter == nil || fake.lastQuery.Filter.Value.String != "alpha" {
		t.Errorf("filter not forwarded: %+v", fake.lastQuery.Filter)
	}
}

func TestHandler_POST_AggregationOnly_202_ReturnsAggShape(t *testing.T) {
	fake := &fakeQueryFetcher{
		aggs: map[string]query.AggregationResult{
			"by_team": {DocCount: 10},
		},
	}
	srv := newTestServer(fake)

	body := query.Query{Entity: query.EntityWorkflowRun, Size: -1}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflowRuns", bytes.NewReader(b))
	w := httptest.NewRecorder()
	srv.newMux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %v, want 200", w.Code)
	}
	var resp aggregationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Entity != "workflowRun" {
		t.Errorf("entity = %v, want workflowRun", resp.Entity)
	}
	if resp.Aggregations["by_team"].DocCount != 10 {
		t.Errorf("agg doc_count = %v, want 10", resp.Aggregations["by_team"].DocCount)
	}
}

func TestHandler_MethodNotAllowed_405(t *testing.T) {
	srv := newTestServer(&fakeQueryFetcher{})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/workflowRuns", nil)
	w := httptest.NewRecorder()
	srv.newMux().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %v, want 405", w.Code)
	}
}

func TestHandler_EmptyResult_ReturnsEmptyArray(t *testing.T) {
	fake := &fakeQueryFetcher{workflowRuns: nil}
	srv := newTestServer(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflowRuns", nil)
	w := httptest.NewRecorder()
	srv.newMux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %v, want 200", w.Code)
	}
	var resp hitsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	// Hits must encode as [] not null
	raw, _ := json.Marshal(resp.Hits)
	if string(raw) == "null" {
		t.Errorf("hits should not encode as null when empty")
	}
}

func ptr[T any](v T) *T { return &v }
