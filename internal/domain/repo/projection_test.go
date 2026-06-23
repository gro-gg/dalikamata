package repo_test

import (
	"context"
	"testing"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/domain/model"
	"github.com/matryer/is"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
	"codeberg.org/aeforged/dalikamata/internal/domain/repo"
)

// TestProjectPullRequest_CycleTimeSecondsTerminal verifies that cycle_time_seconds
// is calculated as updatedAt - createdAt for terminal-state PRs.
func TestProjectPullRequest_CycleTimeSecondsTerminal(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC) // 7 days later

	fixedNow := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC) // far future — must not be used
	r := repo.NewMemory(repo.WithClock(func() time.Time { return fixedNow }))

	is.NoErr(r.AddPullRequest(ctx, model.PullRequest{
		ID:        "pr1",
		State:     model.PullRequestStateMerged,
		CreatedAt: created,
		UpdatedAt: updated,
	}))

	var got []model.PullRequest
	is.NoErr(r.QueryPullRequests(ctx, query.Query{Entity: query.EntityPullRequest}, func(pr model.PullRequest) error {
		got = append(got, pr)
		return nil
	}))
	is.Equal(len(got), 1)

	// Verify via Aggregate that the computed field equals updated - created.
	result, err := r.Aggregate(ctx, query.Query{
		Entity: query.EntityPullRequest,
		Aggs: map[string]query.Aggregation{
			"cycle": {Op: query.AggHistogram, Field: query.PRCycleTimeSeconds, Buckets: []float64{1e9}},
		},
	})
	is.NoErr(err)
	expected := updated.Sub(created).Seconds()
	is.Equal(result["cycle"].Sum, expected)
}

// TestProjectPullRequest_CycleTimeSecondsOpen verifies that cycle_time_seconds
// uses the pinned clock for OPEN PRs.
func TestProjectPullRequest_CycleTimeSecondsOpen(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	fixedNow := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC) // 2 days later

	r := repo.NewMemory(repo.WithClock(func() time.Time { return fixedNow }))
	is.NoErr(r.AddPullRequest(ctx, model.PullRequest{
		ID:        "pr1",
		State:     model.PullRequestStateOpen,
		CreatedAt: created,
		UpdatedAt: created, // open, so updated doesn't matter
	}))

	result, err := r.Aggregate(ctx, query.Query{
		Entity: query.EntityPullRequest,
		Aggs: map[string]query.Aggregation{
			"cycle": {Op: query.AggHistogram, Field: query.PRCycleTimeSeconds, Buckets: []float64{1e9}},
		},
	})
	is.NoErr(err)
	expected := fixedNow.Sub(created).Seconds()
	is.Equal(result["cycle"].Sum, expected)
}

// TestProjectPullRequest_DeclinedUseUpdatedAt verifies DECLINED follows the
// terminal-state path (updatedAt - createdAt), not the OPEN path.
func TestProjectPullRequest_DeclinedUseUpdatedAt(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)
	fixedNow := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	r := repo.NewMemory(repo.WithClock(func() time.Time { return fixedNow }))
	is.NoErr(r.AddPullRequest(ctx, model.PullRequest{
		ID:        "pr1",
		State:     model.PullRequestStateDeclined,
		CreatedAt: created,
		UpdatedAt: updated,
	}))

	result, err := r.Aggregate(ctx, query.Query{
		Entity: query.EntityPullRequest,
		Aggs: map[string]query.Aggregation{
			"cycle": {Op: query.AggHistogram, Field: query.PRCycleTimeSeconds, Buckets: []float64{1e9}},
		},
	})
	is.NoErr(err)
	is.Equal(result["cycle"].Sum, updated.Sub(created).Seconds())
}
