package repo_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/matryer/is"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/internal/domain/repo"
	"codeberg.org/aeforged/dalikamata/pkg/model"
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
