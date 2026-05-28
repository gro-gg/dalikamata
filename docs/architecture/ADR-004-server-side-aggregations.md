# ADR-004: Server-Side Aggregations

**Status:** Accepted  
**Date:** 2026-05-28  
**Supersedes:** None (extends ADR-003)

---

## Context

The `pr_cycle_time_seconds` Prometheus histogram was computed client-side in the metrics service: on every scrape, all pull requests were fetched over NATS, projected and bucketed in-memory (`internal/metrics/service.go`). This had two problems:

1. **O(N) bytes on every scrape** — the full PR payload crossed NATS and was re-allocated on every Prometheus scrape.
2. **Lock-in to one consumer** — any future metric needing histogram-style aggregation had to repeat the same scan-and-bucket loop.

ADR-003 explicitly reserved the `"aggs"` JSON key for future aggregation support and noted the ES-flavoured query DSL as the natural home.

---

## Decision

Extend the query DSL with a server-side aggregation framework and move histogram computation into the domain service. The metrics service becomes a thin client that issues one aggregation request per scrape and walks the result tree to emit Prometheus histograms.

---

## Aggregation DSL

### `Aggregation` type (`internal/domain/query/aggregation.go`)

Mirrors the flat-discriminator style of `Filter`. Unknown ops are always errors — adapters must never silently drop results.

```go
type AggregationOp string

const (
    AggTerms         AggregationOp = "terms"
    AggHistogram     AggregationOp = "histogram"
    AggDateHistogram AggregationOp = "date_histogram"
)

type Aggregation struct {
    Op       AggregationOp          `json:"op"`
    Field    string                 `json:"field,omitempty"`
    Buckets  []float64              `json:"buckets,omitempty"`   // histogram: explicit upper bounds
    Interval string                 `json:"interval,omitempty"` // date_histogram: "month"|"day"|"hour"
    Format   string                 `json:"format,omitempty"`   // date_histogram: Go time layout
    Aggs     map[string]Aggregation `json:"aggs,omitempty"`     // sub-aggregations
}
```

`Query.Aggs map[string]Aggregation` added to the top-level `Query` struct. The `"aggs"` key was already reserved by ADR-003 for exactly this purpose; the addition is wire-compatible.

`Query.Size = -1` is a convention meaning "aggregations only, no hits". Send such queries to `SubjectQueryAggregate` (see below).

### `AggregationResult` type (`internal/domain/query/result.go`)

```go
type AggregationResult struct {
    Buckets  []Bucket `json:"buckets,omitempty"`
    DocCount uint64   `json:"doc_count,omitempty"` // histogram leaf: total observations
    Sum      float64  `json:"sum,omitempty"`       // histogram leaf: sum of observed values
}

type Bucket struct {
    Key      any                          `json:"key"`      // string | float64
    DocCount uint64                       `json:"doc_count"`
    Aggs     map[string]AggregationResult `json:"aggs,omitempty"`
}
```

`Bucket.Key` is always a JSON-safe type:

| Op              | Key type | Semantics                          |
|-----------------|----------|------------------------------------|
| `terms`         | string   | field value (`fmt.Sprint(val)`)    |
| `date_histogram`| string   | formatted interval start           |
| `histogram`     | float64  | explicit upper bound               |

Histogram buckets carry cumulative counts (observations ≤ upper bound), matching `prometheus.MustNewConstHistogram` semantics. `AggregationResult.DocCount` and `.Sum` hold the totals for the leaf.

---

## Computed fields at projection time

The domain query evaluator already consumes `map[string]any` produced by per-entity projection functions in `internal/domain/repo/projection.go`. Computed fields are materialized into that map at read time — downstream code (filter, sort, aggregator) cannot distinguish stored from computed.

**`cycle_time_seconds`** (`query.PRCycleTimeSeconds`) is computed by:

```go
func prCycleTimeSeconds(pr model.PullRequest, now time.Time) float64 {
    end := now
    switch pr.State {
    case model.PullRequestStateMerged, model.PullRequestStateDeclined:
        end = pr.UpdatedAt
    }
    return end.Sub(pr.CreatedAt).Seconds()
}
```

`MemoryRepository` accepts a `WithClock(func() time.Time)` option (default `time.Now`) so tests can pin the clock for deterministic results.

**Extension contract:** adding a new computed field requires:
1. A name constant in `internal/domain/query/fields.go`.
2. One line in the entity's `projectX` function in `internal/domain/repo/projection.go`.

No DSL, wire, or client changes are needed.

---

## NATS wire format

A dedicated subject `query.aggregate` carries aggregation requests. This is intentionally separate from the six per-entity query subjects so existing clients that only handle `data`/`done`/`error` are completely unaffected.

**New header value:** `Daka-Query-Status: aggregation`

Reply sequence for an aggregation request:

```
client → PublishRequest("query.aggregate", inbox, JSON(Query))

server → RespondMsg(inbox, {Daka-Query-Status: aggregation}, JSON(map[string]AggregationResult))
server → RespondMsg(inbox, {Daka-Query-Status: done})
```

On error:
```
server → RespondMsg(inbox, {Daka-Query-Status: error}, {"error":"..."})
```

The `aggregation` message is always a single reply (aggregation results are small compared to hit streams). The existing `data` stream plus a trailing `aggregation` + `done` is reserved for a future combined hits-and-aggs subject.

---

## Layer changes

| Layer | Change |
|-------|--------|
| `internal/domain/query/` | New `aggregation.go`, `result.go`, `aggregator.go`; extend `query.go`, `fields.go` |
| `internal/domain/repo/projection.go` | `projectPullRequest(pr, now)` adds `cycle_time_seconds`; clock seam on `MemoryRepository` |
| `internal/domain/repo/memory.go` | `Aggregate` method; `WithClock` option |
| `internal/domain/ports.go` | `Aggregate` added to `QueryRepository` |
| `internal/domain/service.go` | `Aggregate` added to `QueryHandler`; forwarded by `DomainService` |
| `internal/domain/nats/subjects.go` | `SubjectQueryAggregate`, `StatusAggregation` |
| `internal/domain/nats/query_port.go` | `handleAggregate`, subscribe to new subject |
| `internal/domain/nats/query_client.go` | `Aggregate` method |
| `internal/metrics/service.go` | `PullRequestAggregator` port; rewrote `Collect` as tree walk |

---

## Consequences

**Positive:**
- Metrics service issues O(1) NATS round-trips per scrape regardless of PR count.
- Server-side aggregations are reusable: future metrics add `Aggs` to a `Query` without touching the domain or NATS layer.
- Computed fields (cycle time, future build duration) are a single-line addition per entity.

**Negative / known constraints:**
- `MemoryRepository.Aggregate` is still O(N) — it scans all entities for each aggregation request. A future SQL adapter should push aggregations to the database via `GROUP BY` / `SELECT` expressions.
- Histogram sub-aggregations are not supported in v1 (the aggregator returns an error if `Aggs` is set on a histogram node).
- The `date_histogram` field must already be a `time.Time` in the projected map; a string date field would require a new projected computed field.

---

## Extension contract for future agg ops

1. Add a new `AggregationOp` constant.
2. Add an `applyX` function in `aggregator.go`.
3. Add a case in `applyAgg`.
4. If the op needs new fields on `Aggregation`, add them (the flat-discriminator struct is additive — unused fields are omitted by `omitempty`).

Old clients receive unknown `op` values as a server error, which is correct: they should not silently produce wrong metrics.
