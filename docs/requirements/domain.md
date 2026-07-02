# Domain Service — Reference

The domain service is the central state store. It:

1. Runs an embedded NATS JetStream server (or connects to an external one).
2. Consumes all `ingest.>` events from the `INGEST` stream via durable consumers.
3. Persists entities in `MemoryRepository`.
4. Answers typed queries from consumers (metrics service, API service) over NATS request-reply.

Architecture decisions: [ADR-001](../architecture/ADR-001-microservices-event-driven.md), [ADR-002](../architecture/ADR-002-onion-architecture.md), [ADR-003](../architecture/ADR-003-domain-query-dsl.md), [ADR-004](../architecture/ADR-004-server-side-aggregations.md), [ADR-005](../architecture/ADR-005-component-config.md).

---

## Ingest stream

| Attribute | Value |
|---|---|
| Name | `INGEST` |
| Subject filter | `ingest.>` |
| Storage | File (JetStream persistent) |

### Subjects

| Subject | Payload | Publisher |
|---|---|---|
| `ingest.git.repo` | `model.Repo` JSON | `domain.GitPublisher` |
| `ingest.git.commit` | `model.Commit` JSON | `domain.GitPublisher` |
| `ingest.git.pullrequest` | `model.PullRequest` JSON | `domain.GitPublisher` |
| `ingest.cicd.workflow` | `model.Workflow` JSON | `domain.CICDPublisher` |
| `ingest.cicd.workflowRun` | `model.WorkflowRun` JSON | `domain.CICDPublisher` |
| `ingest.cicd.workflowTask` | `model.WorkflowTask` JSON | `domain.CICDPublisher` |
| `ingest.platform.team` | `model.Team` JSON | `domain.PlatformPublisher` |
| `ingest.platform.component` | `model.Component` JSON | `domain.PlatformPublisher` |
| `ingest.platform.repo` | `model.RepoOnboarding` JSON | `domain.PlatformPublisher` |

Publisher interfaces are defined in `internal/domain/ports.go`. NATS implementations are in `internal/domain/nats/publisher.go`.

### Durable consumers

One durable consumer per subject. All consumers use explicit ack policy and `MaxDeliver=5`. Messages that fail deserialisation are negatively acknowledged and redelivered; after 5 attempts they are not redelivered further.

---

## Query API

Consumers issue typed queries over **core NATS request-reply** (not JetStream). The domain's `QueryPort` subscribes to all `query.*` subjects and responds on the request's per-request inbox.

### Query subjects

| Subject | Entity |
|---|---|
| `query.git.repo` | `model.Repo` |
| `query.git.commit` | `model.Commit` |
| `query.git.pullrequest` | `model.PullRequest` |
| `query.cicd.workflow` | `model.Workflow` |
| `query.cicd.workflowRun` | `model.WorkflowRun` |
| `query.cicd.workflowTask` | `model.WorkflowTask` |
| `query.platform.team` | `model.Team` |
| `query.platform.component` | `model.Component` |
| `query.aggregate` | aggregation results (any entity) |

### Request format

Send a JSON-encoded `query.Query` as the request body. See `internal/domain/query/` for the full DSL.

```json
{
  "entity": "commit",
  "filter": {
    "op": "bool",
    "must": [
      {"op": "term", "field": "repo_id", "value": {"kind": "string", "string": "PROJ/repo"}},
      {"op": "range", "field": "timestamp", "range": {
        "gte": {"kind": "time", "time": "2024-01-01T00:00:00Z"}
      }}
    ]
  },
  "sort": [{"field": "timestamp", "order": "desc"}],
  "from": 0,
  "size": 10
}
```

Supported filter operators: `bool` (must/must_not/should), `term`, `terms`, `range`, `exists`.

### Reply protocol

| `Daka-Query-Status` header | Body |
|---|---|
| `data` | One JSON-encoded entity |
| `done` | Empty — stream terminated cleanly |
| `error` | `{"error": "..."}` — query failed |
| `aggregation` | JSON `map[string]AggregationResult` (aggregation requests only) |

A `query.aggregate` request receives one `aggregation` message then `done`.

### Go client

```go
client := dalinats.NewQueryClient(nc, logger)

// Streaming (good for large result sets):
out, errs := client.QueryCommits(ctx, query.Query{Entity: query.EntityCommit, ...})
for commit := range out { ... }
if err := <-errs; err != nil { ... }

// Collecting (loads all results into a slice):
commits, err := client.QueryCommitsAll(ctx, q)

// Aggregation:
result, err := client.Aggregate(ctx, q)
```

---

## Storage

By default the domain uses `MemoryRepository` — all entity state is held in memory and lost on restart. Pass `--db-path` to activate `SQLiteRepository`, a file-backed persistent store using a pure-Go SQLite driver (no CGo). Both implementations satisfy `domain.Repository` and `domain.QueryRepository` and are interchangeable without any changes to the domain layer.

```bash
dalikamata domain --db-path ./data/dalikamata.db
```

The `docker-compose-micro-persist.yaml` overlay adds `--db-path` and a bind mount for the micro deployment:

```bash
docker compose -f deploy/docker/docker-compose-micro.yaml \
               -f deploy/docker/docker-compose-micro-persist.yaml up
```

## Configuration

| Flag | Default | Description |
|---|---|---|
| `--nats-host` | `0.0.0.0` | NATS server bind address (nats/mono commands) |
| `--nats-port` | `4222` | NATS server port |
| `--nats-data` | `./data/nats` | JetStream persistence directory (nats/mono commands) |
| `--nats-url` | `localhost` | NATS server host to connect to (domain/metrics/api/ingest commands) |
| `--db-path` | _(empty)_ | SQLite database file for persistent entity storage; empty = in-memory (domain/mono commands) |

---

## Error handling

- NATS server startup failure → process aborts.
- NATS client connection failure → process aborts.
- Message deserialisation failure → negative ack; redelivered up to `MaxDeliver` (5) times.
- Unsupported query operator → `error` reply; the server never silently drops results.
