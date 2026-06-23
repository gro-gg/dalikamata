package repo_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	"github.com/matryer/is"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/internal/domain/repo"
)

func newSQLite(t *testing.T, opts ...repo.SQLiteRepositoryOpt) *repo.SQLiteRepository {
	t.Helper()
	r, err := repo.NewSQLite(filepath.Join(t.TempDir(), "test.db"), opts...)
	if err != nil {
		t.Fatalf("opening sqlite repo: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestSQLite_AddAndQueryCommits_FilterByRepoID(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newSQLite(t)

	is.NoErr(r.AddCommit(ctx, model.Commit{SHA: "a", RepoID: "X/repo", Author: "alice", Timestamp: time.Now()}))
	is.NoErr(r.AddCommit(ctx, model.Commit{SHA: "b", RepoID: "Y/repo", Author: "bob", Timestamp: time.Now()}))
	is.NoErr(r.AddCommit(ctx, model.Commit{SHA: "c", RepoID: "X/repo", Author: "carol", Timestamp: time.Now()}))

	q := query.Query{
		Entity: query.EntityCommit,
		Filter: &query.Filter{Op: query.OpTerm, Field: query.CommitRepoID, Value: ptr(query.StringValue("X/repo"))},
	}
	var got []model.Commit
	is.NoErr(r.QueryCommits(ctx, q, func(c model.Commit) error {
		got = append(got, c)
		return nil
	}))
	is.Equal(len(got), 2)
	for _, c := range got {
		is.Equal(c.RepoID, "X/repo")
	}
}

func TestSQLite_SortByTimestampDesc(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newSQLite(t)

	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)
	is.NoErr(r.AddCommit(ctx, model.Commit{SHA: "a", RepoID: "X", Timestamp: t1}))
	is.NoErr(r.AddCommit(ctx, model.Commit{SHA: "b", RepoID: "X", Timestamp: t2}))
	is.NoErr(r.AddCommit(ctx, model.Commit{SHA: "c", RepoID: "X", Timestamp: t0}))

	q := query.Query{
		Entity: query.EntityCommit,
		Sort:   []query.SortField{{Field: query.CommitTimestamp, Order: query.SortDesc}},
	}
	var order []string
	is.NoErr(r.QueryCommits(ctx, q, func(c model.Commit) error {
		order = append(order, c.SHA)
		return nil
	}))
	is.Equal(order, []string{"b", "a", "c"})
}

// TestSQLite_Upsert verifies that re-adding an entity with the same key
// overwrites the previous value rather than duplicating it.
func TestSQLite_Upsert(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newSQLite(t)

	is.NoErr(r.AddRepo(ctx, model.Repo{RepoID: "X/repo", Name: "old"}))
	is.NoErr(r.AddRepo(ctx, model.Repo{RepoID: "X/repo", Name: "new"}))

	var got []model.Repo
	is.NoErr(r.QueryRepos(ctx, query.Query{Entity: query.EntityRepo}, func(v model.Repo) error {
		got = append(got, v)
		return nil
	}))
	is.Equal(len(got), 1)
	is.Equal(got[0].Name, "new")
}

// TestSQLite_Persistence verifies data survives closing and reopening the DB.
func TestSQLite_Persistence(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "persist.db")

	r1, err := repo.NewSQLite(path)
	is.NoErr(err)
	is.NoErr(r1.AddPullRequest(ctx, model.PullRequest{
		ID: "PROJ/repo/1", RepoID: "PROJ/repo", State: model.PullRequestStateMerged,
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	}))
	is.NoErr(r1.Close())

	r2, err := repo.NewSQLite(path)
	is.NoErr(err)
	defer func() { _ = r2.Close() }()

	var got []model.PullRequest
	is.NoErr(r2.QueryPullRequests(ctx, query.Query{Entity: query.EntityPullRequest}, func(pr model.PullRequest) error {
		got = append(got, pr)
		return nil
	}))
	is.Equal(len(got), 1)
	is.Equal(got[0].ID, "PROJ/repo/1")
	is.Equal(got[0].State, model.PullRequestStateMerged)
	is.True(got[0].CreatedAt.Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)))
}

// TestSQLite_PRCycleTimeClock verifies the injected clock drives cycle_time for
// OPEN pull requests via a range filter.
func TestSQLite_PRCycleTimeClock(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	now := time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC) // 1h after creation
	r := newSQLite(t, repo.WithSQLiteClock(func() time.Time { return now }))

	is.NoErr(r.AddPullRequest(ctx, model.PullRequest{
		ID: "p1", State: model.PullRequestStateOpen,
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}))

	result, err := r.Aggregate(ctx, query.Query{
		Entity: query.EntityPullRequest,
		AggsOnly: true,
		Aggs: map[string]query.Aggregation{
			"buckets": {Op: query.AggHistogram, Field: query.PRCycleTimeSeconds, Buckets: []float64{1800, 7200}},
		},
	})
	is.NoErr(err)
	// 3600s cycle time falls in the 7200 bucket, not the 1800 bucket.
	bs := result["buckets"].Buckets
	is.Equal(len(bs), 2)
	is.Equal(bs[0].DocCount, uint64(0)) // <= 1800
	is.Equal(bs[1].DocCount, uint64(1)) // <= 7200
}

// TestSQLite_OwnershipEnrichment verifies WorkflowRun queries surface
// team/component/workflow names rebuilt from the components+workflows tables,
// including the component-overwrite-shrinks-list behaviour.
func TestSQLite_OwnershipEnrichment(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newSQLite(t)

	is.NoErr(r.AddWorkflow(ctx, model.Workflow{ID: "wf1", Name: "Build Pipeline", RepoID: "r1"}))
	is.NoErr(r.AddWorkflow(ctx, model.Workflow{ID: "wf2", Name: "Deploy", RepoID: "r2"}))
	is.NoErr(r.AddWorkflowRun(ctx, model.WorkflowRun{ID: "run1", WorkflowID: "wf1", Status: "SUCCESS"}))
	is.NoErr(r.AddWorkflowRun(ctx, model.WorkflowRun{ID: "run2", WorkflowID: "wf2", Status: "SUCCESS"}))
	is.NoErr(r.AddComponent(ctx, model.Component{
		Name: "svc-a", TeamName: "team-alpha",
		RepoIDs: []string{"r1", "r2"},
	}))

	enriched := map[string]model.WorkflowRun{}
	is.NoErr(r.QueryWorkflowRuns(ctx, query.Query{Entity: query.EntityWorkflowRun}, func(run model.WorkflowRun) error {
		enriched[run.ID] = run
		return nil
	}))
	is.Equal(enriched["run1"].TeamName, "team-alpha")
	is.Equal(enriched["run1"].ComponentName, "svc-a")
	is.Equal(enriched["run1"].WorkflowName, "Build Pipeline")
	is.Equal(enriched["run2"].TeamName, "team-alpha")

	// Re-ingest the component with a shrunk repo list: wf2 (via r2) is now orphaned.
	is.NoErr(r.AddComponent(ctx, model.Component{
		Name: "svc-a", TeamName: "team-alpha",
		RepoIDs: []string{"r1"},
	}))
	enriched = map[string]model.WorkflowRun{}
	is.NoErr(r.QueryWorkflowRuns(ctx, query.Query{Entity: query.EntityWorkflowRun}, func(run model.WorkflowRun) error {
		enriched[run.ID] = run
		return nil
	}))
	is.Equal(enriched["run1"].TeamName, "team-alpha")
	is.Equal(enriched["run2"].TeamName, "unknown")
	is.Equal(enriched["run2"].ComponentName, "unknown")
}

// TestSQLite_ComponentRoundTrip verifies the JSON-encoded nested slices survive
// a write/read cycle.
func TestSQLite_ComponentRoundTrip(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newSQLite(t)

	want := model.Component{
		Name:     "svc",
		TeamName: "team",
		RepoIDs:  []string{"r1", "r2"},
	}
	is.NoErr(r.AddComponent(ctx, want))

	var got []model.Component
	is.NoErr(r.QueryComponents(ctx, query.Query{Entity: query.EntityComponent}, func(c model.Component) error {
		got = append(got, c)
		return nil
	}))
	is.Equal(len(got), 1)
	is.Equal(got[0], want)
}
