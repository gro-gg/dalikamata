# Query Guide

How to write queries against the domain service using the Go query DSL.

The design rationale lives in [ADR-003](architecture/ADR-003-domain-query-dsl.md) and
[ADR-004](architecture/ADR-004-server-side-aggregations.md). This page is a practical
reference: one example per operator, a field table per entity, and the full aggregation
syntax.

---

## Getting a QueryClient

Wire up a `QueryClient` from an existing `*nats.Conn`. The client is safe for concurrent use.

```go
import (
    gonats "github.com/nats-io/nats.go"
    dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
)

nc, err := gonats.Connect("nats://localhost:4222")
// handle err

client := dalinats.NewQueryClient(nc, logger)
// or with a custom timeout (default is 30s):
client = dalinats.NewQueryClient(nc, logger, dalinats.WithQueryTimeout(10*time.Second))
```

The domain service must be running (`dalikamata domain` or `dalikamata mono`) for queries to succeed.

---

## Fetching entities

Each entity type has two methods on `QueryClient`:

| Method | Returns |
|---|---|
| `QueryXxxAll(ctx, q)` | `([]model.Xxx, error)` — collects all matches into a slice |
| `QueryXxx(ctx, q)` | `(<-chan model.Xxx, <-chan error)` — streams results for large result sets |

**Fetch everything** (no filter):

```go
prs, err := client.QueryPullRequestsAll(ctx, query.Query{
    Entity: query.EntityPullRequest,
})
```

**Stream large result sets** — read from the channel until it closes, then check the error channel:

```go
out, errs := client.QueryCommits(ctx, query.Query{Entity: query.EntityCommit})
for commit := range out {
    fmt.Println(commit.SHA)
}
if err := <-errs; err != nil {
    log.Fatal(err)
}
```

---

## Filter operators

All imports below assume:

```go
import "codeberg.org/aeforged/dalikamata/internal/domain/query"
```

### term — exact match

```go
// PRs in a specific repository.
q := query.Query{
    Entity: query.EntityPullRequest,
    Filter: query.Ptr(query.TermFilter(query.PRRepoID, query.StringValue("MYPROJ/my-repo"))),
}
```

### terms — match any value in a set

```go
// Commits by alice or bob.
q := query.Query{
    Entity: query.EntityCommit,
    Filter: query.Ptr(query.TermsFilter(query.CommitAuthor,
        query.StringValue("alice"),
        query.StringValue("bob"),
    )),
}
```

### range — numeric or time bounds

```go
from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
to   := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

// Commits in the first half of 2024.
q := query.Query{
    Entity: query.EntityCommit,
    Filter: query.Ptr(query.RangeFilter(query.CommitTimestamp, query.Range{
        GTE: query.Ptr(query.TimeValue(from)),
        LT:  query.Ptr(query.TimeValue(to)),
    })),
}
```

All four bounds (`GT`, `GTE`, `LT`, `LTE`) are optional; set only the ones you need.

Numeric range (workflow runs that took more than 5 minutes):

```go
q := query.Query{
    Entity: query.EntityWorkflowRun,
    Filter: query.Ptr(query.RangeFilter(query.RunDuration, query.Range{
        GT: query.Ptr(query.FloatValue(300)),
    })),
}
```

### exists — field is present

```go
// Commits that carry an author field.
q := query.Query{
    Entity: query.EntityCommit,
    Filter: query.Ptr(query.ExistsFilter(query.CommitAuthor)),
}
```

---

## Combining filters

### AND — all conditions must hold

```go
// Merged PRs in a specific repo.
q := query.Query{
    Entity: query.EntityPullRequest,
    Filter: query.Ptr(query.AndFilter(
        query.TermFilter(query.PRRepoID, query.StringValue("MYPROJ/my-repo")),
        query.TermFilter(query.PRState,  query.StringValue("MERGED")),
    )),
}
```

### OR — at least one condition must hold

```go
// PRs that are either merged or declined (same as TermsFilter for a single field).
q := query.Query{
    Entity: query.EntityPullRequest,
    Filter: query.Ptr(query.OrFilter(
        query.TermFilter(query.PRState, query.StringValue("MERGED")),
        query.TermFilter(query.PRState, query.StringValue("DECLINED")),
    )),
}
```

### NOT — exclude matching documents

```go
// Workflow runs that are not on the main branch.
q := query.Query{
    Entity: query.EntityWorkflowRun,
    Filter: query.Ptr(query.NotFilter(
        query.TermFilter(query.RunBranch, query.StringValue("main")),
    )),
}
```

### Nesting — AND inside NOT, OR inside AND, etc.

Filters compose arbitrarily. `AndFilter`, `OrFilter`, and `NotFilter` each accept
any `Filter` value, including other composed filters:

```go
// (state=MERGED OR state=DECLINED) AND repo=MYPROJ/repo AND NOT author=bot
q := query.Query{
    Entity: query.EntityPullRequest,
    Filter: query.Ptr(query.AndFilter(
        query.OrFilter(
            query.TermFilter(query.PRState, query.StringValue("MERGED")),
            query.TermFilter(query.PRState, query.StringValue("DECLINED")),
        ),
        query.TermFilter(query.PRRepoID, query.StringValue("MYPROJ/repo")),
        query.NotFilter(
            query.TermFilter(query.PRAuthor, query.StringValue("bot")),
        ),
    )),
}
```

For full control over `Must` + `Should` in a single `bool` node (e.g. boosting), use
the `query.Filter` struct literal directly:

```go
filter := query.Filter{
    Op:   query.OpBool,
    Must: []query.Filter{query.TermFilter(query.PRRepoID, query.StringValue("X/repo"))},
    Should: []query.Filter{query.TermFilter(query.PRAuthor, query.StringValue("alice"))},
}
```

---

## Sort and pagination

```go
q := query.Query{
    Entity: query.EntityCommit,
    // Sort by timestamp descending, then by SHA ascending as a tiebreaker.
    Sort: []query.SortField{
        {Field: query.CommitTimestamp, Order: query.SortDesc},
        {Field: query.CommitSHA,       Order: query.SortAsc},
    },
    // Return items 20–29 (zero-based offset, page size 10).
    From: 20,
    Size: 10,
}
```

`Size: 0` (the zero value) returns all matches.

---

## Aggregations

Aggregations run server-side and return a result tree. There are three modes,
controlled by `AggsOnly` and whether `Aggs` is set:

| `AggsOnly` | `Aggs` | Result |
|---|---|---|
| `false` | empty | hits only (default) |
| `false` | non-empty | hits + aggregations in one response |
| `true` | non-empty | aggregations only, no entity hits |

### Aggregations only

Use `AggsOnly: true` when you only need the summary and don't want entity records.
Call `client.Aggregate` — it routes to the dedicated aggregate subject:

```go
result, err := client.Aggregate(ctx, query.Query{
    Entity:   query.EntityPullRequest,
    AggsOnly: true,
    Aggs: map[string]query.Aggregation{
        "by_state": {Op: query.AggTerms, Field: query.PRState},
    },
})

for _, bucket := range result["by_state"].Buckets {
    fmt.Printf("%s: %d\n", bucket.Key, bucket.DocCount)
}
```

### Hits and aggregations together

Omit `AggsOnly` (or set it to `false`) and include a non-empty `Aggs` map. Call the
entity's `QueryXxxAll` method — the aggregation is computed over the full matching set
regardless of `Size`/`From`, and is returned alongside the paginated hits:

```go
prs, err := client.QueryPullRequestsAll(ctx, query.Query{
    Entity: query.EntityPullRequest,
    Filter: query.Ptr(query.TermFilter(query.PRRepoID, query.StringValue("MYPROJ/repo"))),
    Sort:   []query.SortField{{Field: query.PRCreatedAt, Order: query.SortDesc}},
    Size:   20,
    Aggs: map[string]query.Aggregation{
        "by_state": {Op: query.AggTerms, Field: query.PRState},
    },
})
// prs contains the first 20 matching pull requests.
// To get the aggregation alongside hits, use the HTTP POST API or extend
// the domain service to return both — the QueryClient's streaming methods
// return entity records only.
```

> **Note:** the Go `QueryClient` methods return entity records only. The combined
> hits + aggregation response is available via the HTTP API (POST to any entity
> endpoint with `"aggs_only": false` and a non-empty `"aggs"` map). If you need
> both in Go without going through HTTP, call `QueryXxxAll` and `Aggregate`
> sequentially.

### Date histogram

PRs grouped by the month they were created:

```go
result, err := client.Aggregate(ctx, query.Query{
    Entity:   query.EntityPullRequest,
    AggsOnly: true,
    Aggs: map[string]query.Aggregation{
        "by_month": {
            Op:       query.AggDateHistogram,
            Field:    query.PRCreatedAt,
            Interval: "month",
            Format:   "2006-01", // Go time layout for the bucket key
        },
    },
})
```

### Numeric histogram

PR cycle times bucketed into Prometheus-style cumulative bins:

```go
result, err := client.Aggregate(ctx, query.Query{
    Entity:   query.EntityPullRequest,
    AggsOnly: true,
    Aggs: map[string]query.Aggregation{
        "cycle_time": {
            Op:      query.AggHistogram,
            Field:   query.PRCycleTimeSeconds,
            Buckets: []float64{3600, 14400, 86400, 259200, 604800}, // 1h 4h 1d 3d 7d
        },
    },
})

hist := result["cycle_time"]
// hist.DocCount — total number of PRs observed
// hist.Sum      — total of all cycle_time_seconds values
// hist.Buckets  — cumulative counts per upper bound (Prometheus histogram format)
```

### Nested aggregations

Sub-aggregations run within each bucket produced by their parent. Nest them via the
`Aggs` field on the parent `Aggregation`. The metrics service uses a 6-level tree to
compute Prometheus histograms per team/component/workflow/branch:

```go
result, err := client.Aggregate(ctx, query.Query{
    Entity:   query.EntityWorkflowRun,
    AggsOnly: true,
    Aggs: map[string]query.Aggregation{
        "by_team": {
            Op:    query.AggTerms,
            Field: query.RunTeamName,
            Aggs: map[string]query.Aggregation{
                "by_branch": {
                    Op:    query.AggTerms,
                    Field: query.RunBranch,
                    Aggs: map[string]query.Aggregation{
                        "duration": {
                            Op:      query.AggHistogram,
                            Field:   query.RunDuration,
                            Buckets: []float64{60, 300, 900, 1800, 3600},
                        },
                    },
                },
            },
        },
    },
})

for _, teamBucket := range result["by_team"].Buckets {
    team := teamBucket.Key.(string)
    for _, branchBucket := range teamBucket.Aggs["by_branch"].Buckets {
        branch := branchBucket.Key.(string)
        hist := branchBucket.Aggs["duration"]
        fmt.Printf("%s / %s: %d runs, total %.0fs\n",
            team, branch, hist.DocCount, hist.Sum)
    }
}
```

### Aggregation with a filter

Filters narrow the entity set before aggregation:

```go
result, err := client.Aggregate(ctx, query.Query{
    Entity:   query.EntityPullRequest,
    AggsOnly: true,
    Filter:   query.Ptr(query.TermFilter(query.PRState, query.StringValue("MERGED"))),
    Aggs: map[string]query.Aggregation{
        "by_repo": {Op: query.AggTerms, Field: query.PRRepoID},
    },
})
```

---

## Field reference

Field name constants live in `internal/domain/query/fields.go`. Use the constants
rather than bare strings — they are stable across refactors.

### Repo

| Constant | JSON field | Type |
|---|---|---|
| `query.RepoID` | `repo_id` | string |
| `query.RepoName` | `name` | string |

### Commit

| Constant | JSON field | Type |
|---|---|---|
| `query.CommitSHA` | `sha` | string |
| `query.CommitRepoID` | `repo_id` | string |
| `query.CommitAuthor` | `author` | string |
| `query.CommitTimestamp` | `timestamp` | time |

### PullRequest

| Constant | JSON field | Type |
|---|---|---|
| `query.PRID` | `id` | string |
| `query.PRRepoID` | `repo_id` | string |
| `query.PRName` | `name` | string |
| `query.PRTitle` | `title` | string |
| `query.PRAuthor` | `author` | string |
| `query.PRState` | `state` | string (`OPEN`, `MERGED`, `DECLINED`) |
| `query.PRCreatedAt` | `created_at` | time |
| `query.PRUpdatedAt` | `updated_at` | time |
| `query.PRCycleTimeSeconds` | `cycle_time_seconds` | float — computed at query time |

### Workflow

| Constant | JSON field | Type |
|---|---|---|
| `query.WorkflowID` | `id` | string |
| `query.WorkflowName` | `name` | string |

A workflow carries `repo_ids` (a list of every repo checked out — app repo plus shared libraries), but it is not a filterable query field: the scalar query engine cannot match against a list (the same reason a Component's `repo_ids` is not filterable).

### WorkflowRun

| Constant | JSON field | Type |
|---|---|---|
| `query.RunID` | `id` | string |
| `query.RunWorkflowID` | `workflow_id` | string |
| `query.RunWorkflowName` | `workflow_name` | string — enriched at query time |
| `query.RunNumber` | `number` | int |
| `query.RunStatus` | `status` | string |
| `query.RunBranch` | `branch` | string |
| `query.RunCommitSHA` | `commit_sha` | string |
| `query.RunStartedAt` | `started_at` | time |
| `query.RunDuration` | `duration` | float (seconds) |
| `query.RunComponentName` | `component_name` | string — enriched at query time |
| `query.RunTeamName` | `team_name` | string — enriched at query time |

### WorkflowTask

| Constant | JSON field | Type |
|---|---|---|
| `query.TaskWorkflowRunID` | `workflow_run_id` | string |
| `query.TaskOrder` | `order` | int — position within the run |
| `query.TaskName` | `name` | string |
| `query.TaskStatus` | `status` | string |
| `query.TaskStartedAt` | `started_at` | time |
| `query.TaskDuration` | `duration` | float (seconds) |
| `query.TaskWorkflowID` | `workflow_id` | string — enriched at query time |
| `query.TaskWorkflowName` | `workflow_name` | string — enriched at query time |
| `query.TaskComponentName` | `component_name` | string — enriched at query time |
| `query.TaskTeamName` | `team_name` | string — enriched at query time |
| `query.TaskBranch` | `branch` | string — enriched at query time |

### Team

| Constant | JSON field | Type |
|---|---|---|
| `query.TeamName` | `name` | string |

### Component

| Constant | JSON field | Type |
|---|---|---|
| `query.ComponentName` | `name` | string |
| `query.ComponentTeamName` | `team_name` | string |
