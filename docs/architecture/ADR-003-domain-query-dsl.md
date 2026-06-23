# ADR-003: Domain Query DSL — typed Go DSL over NATS request-reply

## Status

Accepted

> **Updated by** ADR-004 (server-side aggregations, adds `query.aggregate` subject and `Aggs` field to `Query`), ADR-005 (platform entities, adds `query.platform.team` and `query.platform.component` subjects).

## Context

The domain service has always been write-only from a consumer's perspective. Ingestors publish events into `ingest.>`, the `DomainService` persists them to `MemoryRepository`, and downstream services such as the metrics service re-derive their state by subscribing to the live event stream. This works for a single, simple metric (PR cycle time) but does not generalise:

- Each new metrics generator that needs historical context must either subscribe to the full event stream and replay it on startup, or ask the domain "what do you currently know about X?" — which currently has no mechanism.
- Future aggregation use cases (p95 PR cycle time per repo per month, deployment frequency per workflow, MTTR per branch) require efficient queries over the accumulated entity state, not just event subscriptions.
- The in-memory `MemoryRepository` is already typed by concern (`Repository` for writes); adding reads via a separate port contract is a natural extension.

## Decision

### 1. A typed Go query DSL lives in the domain layer

A new package `internal/domain/query/` defines a typed, JSON-serializable query language modelled after Elasticsearch's Query DSL. The DSL lives in the **domain layer** — it expresses "what can be queried" in domain terms, not storage terms. Adapters are responsible for translating it to their native storage operations.

The top-level type:

```go
type Query struct {
    Entity Entity      `json:"entity"`
    Filter *Filter     `json:"filter,omitempty"`
    Sort   []SortField `json:"sort,omitempty"`
    From   int         `json:"from,omitempty"`
    Size   int         `json:"size,omitempty"` // 0 = return all
}
```

Supported filter operators in v1: `bool` (must/must_not/should), `term`, `terms`, `range`, `exists`. Aggregations are **explicitly out of scope** for v1; the `"aggs"` JSON key is reserved by convention and adding it later is a wire-compatible additive change. (Aggregations were added in ADR-004.)

### 2. Filter tree uses a discriminator rather than ES positional polymorphism

Elasticsearch uses positional polymorphism: `{"range": {"timestamp": {"gte": "..."}}}` — the outer key is the operator. This requires custom `UnmarshalJSON` marshalers in Go to walk the map and dispatch by key.

This DSL uses a flat discriminator instead:

```json
{"op": "range", "field": "timestamp", "range": {"gte": {"kind": "time", "time": "..."}}}
```

This is plain `encoding/json`, costs one extra key per filter node, and is easier to inspect in raw NATS messages. The DSL is internal to dalikamata, so there is no requirement for ES wire-format compatibility.

### 3. Storage is separated from the query interface by port boundaries

Two new port interfaces are defined in `internal/domain/`:

- `QueryRepository` (secondary/driven): the storage adapter implements this. Six `QueryX(ctx, Query, emit func(T) error) error` methods, one per entity. The `emit` callback streams results; returning an error aborts the query.
- `QueryHandler` (primary/driving): `DomainService` implements this. The NATS adapter depends on `QueryHandler`, not on a concrete service or storage type.

The adapter's responsibility is faithful translation. **An adapter must return an error for unsupported operators; it must never silently drop results.** This contract ensures metrics generators can trust the results.

The `MemoryRepository` implements both `Repository` and `QueryRepository`. A future `SQLiteRepository` or `PostgresRepository` would implement the same two interfaces and push filter/sort/limit down to the SQL engine; no changes to the domain layer or the NATS adapter would be required.

### 4. Wire transport: NATS request-many over a per-request inbox

Query requests use **core NATS** request-reply (not JetStream), as request-reply is a core NATS feature. The query path does not require durable consumers or replay.

Subjects mirror the ingest hierarchy under a `query.` prefix:

| Subject | Entity |
|---|---|
| `query.git.repo` | `model.Repo` |
| `query.git.commit` | `model.Commit` |
| `query.git.pullrequest` | `model.PullRequest` |
| `query.cicd.workflow` | `model.Workflow` |
| `query.cicd.workflowRun` | `model.WorkflowRun` |
| `query.cicd.workflowTask` | `model.WorkflowTask` |
| `query.platform.team` | `model.Team` (added in ADR-005) |
| `query.platform.component` | `model.Component` (added in ADR-005) |
| `query.aggregate` | aggregation requests (added in ADR-004) |

Each request body is a JSON-encoded `query.Query`. The server publishes N reply messages to the request's per-request inbox (`msg.Reply`), each with a `Daka-Query-Status` header:

| Header value | Body |
|---|---|
| `data` | One JSON-encoded entity |
| `done` | Empty — stream terminator |
| `error` | `{"error": "..."}` — error terminator |

The client (`QueryClient`) creates a `SubscribeSync` inbox *before* publishing the request to avoid missing fast replies, then reads until `done` or `error`.

### 5. Why Elasticsearch flavor

The DSL uses ES vocabulary (`bool`/`must`/`should`, `term`, `range`, `terms`, `aggs`) because the aggregation primitives (terms, date_histogram, percentiles) map cleanly to future SQL/analytics backends and the vocabulary is widely known. The `"aggs"` key was reserved at v1 so that ADR-004's aggregation extension required no breaking wire change.

## Consequences

- The domain service now runs **two NATS ports** concurrently: the JetStream ingest port (existing) and the core NATS query port (new). Both share the same `*nats.Conn`.
- Queries on the `MemoryRepository` are **O(N)** — a full scan per query. This is acceptable at current data volumes. A SQL-backed adapter pushes filter/sort/limit to the engine and removes this constraint.
- The existing PR cycle-time metric service is **not affected** — it remains event-driven and does not use the query API. Porting it is a follow-up task.
- Per-request inbox semantics offer no backpressure. If a client is slow to consume replies, the server's outbound NATS buffer will fill and `PublishMsg` will return `ErrSlowConsumer`, aborting the query. Clients should consume results promptly.
- Aggregations (`aggs`) are not supported in v1. The server ignores the reserved `"aggs"` key if present.
- A SQLite adapter (`internal/domain/repo/sqlite.go`) implements `Repository` and `QueryRepository` and is activated with `--db-path`. A PostgreSQL adapter would follow the same pattern.

## Code locations

| Concern | Location |
|---|---|
| DSL types | `internal/domain/query/` |
| Port interfaces | `internal/domain/ports.go` |
| MemoryRepository readers | `internal/domain/repo/memory.go`, `projection.go` |
| DomainService forwarders | `internal/domain/service.go` |
| NATS server adapter | `internal/domain/nats/query_port.go` |
| NATS client library | `internal/domain/nats/query_client.go` |
| Subject constants | `internal/domain/nats/subjects.go` |
| App wiring | `internal/app/domain.go` |
