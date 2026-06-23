package repo_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	"github.com/matryer/is"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/internal/domain/repo"
)

func ptr[T any](v T) *T { return &v }

func newRepo() *repo.MemoryRepository {
	return repo.NewMemory()
}

// ---- QueryCommits ----------------------------------------------------------

func TestQueryCommits_FilterByRepoID(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newRepo()

	is.NoErr(r.AddCommit(ctx, model.Commit{SHA: "a", RepoID: "X/repo", Author: "alice", Timestamp: time.Now()}))
	is.NoErr(r.AddCommit(ctx, model.Commit{SHA: "b", RepoID: "Y/repo", Author: "bob", Timestamp: time.Now()}))
	is.NoErr(r.AddCommit(ctx, model.Commit{SHA: "c", RepoID: "X/repo", Author: "carol", Timestamp: time.Now()}))

	q := query.Query{
		Entity: query.EntityCommit,
		Filter: &query.Filter{
			Op:    query.OpTerm,
			Field: query.CommitRepoID,
			Value: ptr(query.StringValue("X/repo")),
		},
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

func TestQueryCommits_SortByTimestampDesc(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newRepo()

	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)

	is.NoErr(r.AddCommit(ctx, model.Commit{SHA: "a", RepoID: "R", Timestamp: t1}))
	is.NoErr(r.AddCommit(ctx, model.Commit{SHA: "b", RepoID: "R", Timestamp: t0}))
	is.NoErr(r.AddCommit(ctx, model.Commit{SHA: "c", RepoID: "R", Timestamp: t2}))

	q := query.Query{
		Entity: query.EntityCommit,
		Sort:   []query.SortField{{Field: query.CommitTimestamp, Order: query.SortDesc}},
	}

	var got []model.Commit
	is.NoErr(r.QueryCommits(ctx, q, func(c model.Commit) error {
		got = append(got, c)
		return nil
	}))
	is.Equal(len(got), 3)
	is.True(got[0].Timestamp.Equal(t2))
	is.True(got[1].Timestamp.Equal(t1))
	is.True(got[2].Timestamp.Equal(t0))
}

func TestQueryCommits_Pagination(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newRepo()

	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 5 {
		is.NoErr(r.AddCommit(ctx, model.Commit{
			SHA:       string(rune('a' + i)),
			RepoID:    "R",
			Timestamp: t0.Add(time.Duration(i) * time.Hour),
		}))
	}

	q := query.Query{
		Entity: query.EntityCommit,
		Sort:   []query.SortField{{Field: query.CommitTimestamp, Order: query.SortAsc}},
		From:   1,
		Size:   2,
	}

	var got []model.Commit
	is.NoErr(r.QueryCommits(ctx, q, func(c model.Commit) error {
		got = append(got, c)
		return nil
	}))
	is.Equal(len(got), 2)
}

// ---- QueryPullRequests -----------------------------------------------------

func TestQueryPullRequests_FilterByState(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newRepo()

	is.NoErr(r.AddPullRequest(ctx, model.PullRequest{ID: "1", State: model.PullRequestStateMerged, RepoID: "R"}))
	is.NoErr(r.AddPullRequest(ctx, model.PullRequest{ID: "2", State: model.PullRequestStateOpen, RepoID: "R"}))
	is.NoErr(r.AddPullRequest(ctx, model.PullRequest{ID: "3", State: model.PullRequestStateDeclined, RepoID: "R"}))

	q := query.Query{
		Entity: query.EntityPullRequest,
		Filter: &query.Filter{
			Op:    query.OpTerms,
			Field: query.PRState,
			Values: []query.Value{
				query.StringValue(model.PullRequestStateMerged),
				query.StringValue(model.PullRequestStateDeclined),
			},
		},
	}

	var got []model.PullRequest
	is.NoErr(r.QueryPullRequests(ctx, q, func(pr model.PullRequest) error {
		got = append(got, pr)
		return nil
	}))
	is.Equal(len(got), 2)
}

// ---- QueryWorkflowRuns -----------------------------------------------------

func TestQueryWorkflowRuns_RangeOnDuration(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newRepo()

	for i, dur := range []float64{10, 30, 90, 120} {
		is.NoErr(r.AddWorkflowRun(ctx, model.WorkflowRun{
			ID:         string(rune('a' + i)),
			WorkflowID: "W",
			Duration:   dur,
			Status:     model.BuildStatusSuccess,
		}))
	}

	q := query.Query{
		Entity: query.EntityWorkflowRun,
		Filter: &query.Filter{
			Op:    query.OpRange,
			Field: query.RunDuration,
			Range: &query.Range{
				GTE: ptr(query.FloatValue(30.0)),
				LTE: ptr(query.FloatValue(90.0)),
			},
		},
	}

	var got []model.WorkflowRun
	is.NoErr(r.QueryWorkflowRuns(ctx, q, func(run model.WorkflowRun) error {
		got = append(got, run)
		return nil
	}))
	is.Equal(len(got), 2)
}

// ---- Emit callback stops iteration ----------------------------------------

func TestQueryCommits_EmitErrorStopsIteration(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newRepo()

	for i := range 5 {
		is.NoErr(r.AddCommit(ctx, model.Commit{
			SHA:    string(rune('a' + i)),
			RepoID: "R",
		}))
	}

	sentinel := errors.New("stop")
	var count int
	err := r.QueryCommits(ctx, query.Query{Entity: query.EntityCommit}, func(model.Commit) error {
		count++
		return sentinel
	})
	is.Equal(err, sentinel)
	is.Equal(count, 1) // stopped after first
}

// ---- Aggregate -------------------------------------------------------------

func TestAggregate_PRCycleTimeHistogram(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC) // 1 day = 86400s

	r := repo.NewMemory(repo.WithClock(func() time.Time { return updated }))
	is.NoErr(r.AddPullRequest(ctx, model.PullRequest{
		ID:        "1",
		RepoID:    "R",
		State:     model.PullRequestStateMerged,
		CreatedAt: created,
		UpdatedAt: updated,
	}))

	buckets := []float64{3600, 14400, 86400, 259200, 604800}
	result, err := r.Aggregate(ctx, query.Query{
		Entity: query.EntityPullRequest,
		Aggs: map[string]query.Aggregation{
			"cycle": {Op: query.AggHistogram, Field: query.PRCycleTimeSeconds, Buckets: buckets},
		},
	})
	is.NoErr(err)
	cycle := result["cycle"]
	is.Equal(cycle.DocCount, uint64(1))
	is.Equal(cycle.Sum, 86400.0)
	// 86400 ≤ 86400 → cumulative counts at bounds 86400, 259200, 604800 = 1
	countAt := func(bound float64) uint64 {
		for _, b := range cycle.Buckets {
			if b.Key.(float64) == bound {
				return b.DocCount
			}
		}
		return 0
	}
	is.Equal(countAt(3600), uint64(0))
	is.Equal(countAt(14400), uint64(0))
	is.Equal(countAt(86400), uint64(1))
	is.Equal(countAt(259200), uint64(1))
	is.Equal(countAt(604800), uint64(1))
}

func TestAggregate_NilWhenNoAggs(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newRepo()
	result, err := r.Aggregate(ctx, query.Query{Entity: query.EntityPullRequest})
	is.NoErr(err)
	is.True(result == nil)
}

func TestAggregate_FilterApplied(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newRepo()

	is.NoErr(r.AddPullRequest(ctx, model.PullRequest{ID: "1", RepoID: "A", State: model.PullRequestStateMerged}))
	is.NoErr(r.AddPullRequest(ctx, model.PullRequest{ID: "2", RepoID: "B", State: model.PullRequestStateMerged}))

	result, err := r.Aggregate(ctx, query.Query{
		Entity: query.EntityPullRequest,
		Filter: &query.Filter{
			Op:    query.OpTerm,
			Field: query.PRRepoID,
			Value: ptr(query.StringValue("A")),
		},
		Aggs: map[string]query.Aggregation{
			"by_state": {Op: query.AggTerms, Field: query.PRState},
		},
	})
	is.NoErr(err)
	// Only repo A's PR passes the filter.
	is.Equal(result["by_state"].Buckets[0].DocCount, uint64(1))
}

func TestAggregate_NestedTermsDateHistTermsHistogram(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	jan := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	janUpdated := time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC) // 1 day cycle
	fixed := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	r := repo.NewMemory(repo.WithClock(func() time.Time { return fixed }))
	is.NoErr(r.AddPullRequest(ctx, model.PullRequest{
		ID: "1", RepoID: "R", State: model.PullRequestStateMerged,
		CreatedAt: jan, UpdatedAt: janUpdated,
	}))

	result, err := r.Aggregate(ctx, query.Query{
		Entity: query.EntityPullRequest,
		Aggs: map[string]query.Aggregation{
			"by_repo": {Op: query.AggTerms, Field: query.PRRepoID,
				Aggs: map[string]query.Aggregation{
					"by_month": {Op: query.AggDateHistogram, Field: query.PRCreatedAt, Interval: "month", Format: "2006-01",
						Aggs: map[string]query.Aggregation{
							"by_state": {Op: query.AggTerms, Field: query.PRState,
								Aggs: map[string]query.Aggregation{
									"cycle": {Op: query.AggHistogram, Field: query.PRCycleTimeSeconds, Buckets: []float64{3600, 86400, 604800}},
								},
							},
						},
					},
				},
			},
		},
	})
	is.NoErr(err)

	repoBucket := result["by_repo"].Buckets[0]
	is.Equal(repoBucket.Key, "R")

	monthBucket := repoBucket.Aggs["by_month"].Buckets[0]
	is.Equal(monthBucket.Key, "2024-01")

	stateBucket := monthBucket.Aggs["by_state"].Buckets[0]
	is.Equal(stateBucket.Key, "MERGED")

	cycle := stateBucket.Aggs["cycle"]
	is.Equal(cycle.DocCount, uint64(1))
	is.Equal(cycle.Sum, janUpdated.Sub(jan).Seconds())
}

// ---- Concurrency -----------------------------------------------------------

func TestQueryCommits_ConcurrentReadWrite(t *testing.T) {
	ctx := context.Background()
	r := newRepo()

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = r.AddCommit(ctx, model.Commit{
				SHA:    string(rune('a' + i)),
				RepoID: "R",
			})
		}(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.QueryCommits(ctx, query.Query{Entity: query.EntityCommit}, func(model.Commit) error {
				return nil
			})
		}()
	}
	wg.Wait()
}

// ---- QueryTeams ------------------------------------------------------------

func TestQueryTeams_FilterByName(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newRepo()

	is.NoErr(r.AddTeam(ctx, model.Team{Name: "payments"}))
	is.NoErr(r.AddTeam(ctx, model.Team{Name: "checkout"}))

	q := query.Query{
		Entity: query.EntityTeam,
		Filter: &query.Filter{
			Op:    query.OpTerm,
			Field: query.TeamName,
			Value: ptr(query.StringValue("payments")),
		},
	}
	var got []model.Team
	is.NoErr(r.QueryTeams(ctx, q, func(t model.Team) error {
		got = append(got, t)
		return nil
	}))
	is.Equal(len(got), 1)
	is.Equal(got[0].Name, "payments")
}

func TestQueryTeams_All(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newRepo()

	is.NoErr(r.AddTeam(ctx, model.Team{Name: "alpha"}))
	is.NoErr(r.AddTeam(ctx, model.Team{Name: "beta"}))

	var got []model.Team
	is.NoErr(r.QueryTeams(ctx, query.Query{Entity: query.EntityTeam}, func(t model.Team) error {
		got = append(got, t)
		return nil
	}))
	is.Equal(len(got), 2)
}

// ---- QueryComponents -------------------------------------------------------

func TestQueryComponents_FilterByTeam(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newRepo()

	is.NoErr(r.AddComponent(ctx, model.Component{Name: "payment-service", TeamName: "payments"}))
	is.NoErr(r.AddComponent(ctx, model.Component{Name: "checkout-api", TeamName: "checkout"}))
	is.NoErr(r.AddComponent(ctx, model.Component{Name: "payment-worker", TeamName: "payments"}))

	q := query.Query{
		Entity: query.EntityComponent,
		Filter: &query.Filter{
			Op:    query.OpTerm,
			Field: query.ComponentTeamName,
			Value: ptr(query.StringValue("payments")),
		},
	}
	var got []model.Component
	is.NoErr(r.QueryComponents(ctx, q, func(c model.Component) error {
		got = append(got, c)
		return nil
	}))
	is.Equal(len(got), 2)
	for _, c := range got {
		is.Equal(c.TeamName, "payments")
	}
}

func TestQueryComponents_SortByName(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newRepo()

	is.NoErr(r.AddComponent(ctx, model.Component{Name: "z-service", TeamName: "team"}))
	is.NoErr(r.AddComponent(ctx, model.Component{Name: "a-service", TeamName: "team"}))

	q := query.Query{
		Entity: query.EntityComponent,
		Sort:   []query.SortField{{Field: query.ComponentName, Order: query.SortAsc}},
	}
	var got []model.Component
	is.NoErr(r.QueryComponents(ctx, q, func(c model.Component) error {
		got = append(got, c)
		return nil
	}))
	is.Equal(len(got), 2)
	is.Equal(got[0].Name, "a-service")
	is.Equal(got[1].Name, "z-service")
}

func TestAggregate_Team(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	r := newRepo()

	is.NoErr(r.AddTeam(ctx, model.Team{Name: "alpha"}))
	is.NoErr(r.AddTeam(ctx, model.Team{Name: "beta"}))

	q := query.Query{
		Entity: query.EntityTeam,
		AggsOnly: true,
		Aggs: map[string]query.Aggregation{
			"by_name": {Op: query.AggTerms, Field: query.TeamName},
		},
	}
	result, err := r.Aggregate(ctx, q)
	is.NoErr(err)
	byName, ok := result["by_name"]
	is.True(ok)
	is.Equal(len(byName.Buckets), 2)
}
