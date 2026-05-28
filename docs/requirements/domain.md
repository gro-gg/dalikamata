# Domain Service — Requirements

## Purpose

Provide the event streaming backbone for dalikamata. The domain service runs an embedded NATS JetStream server and manages the ingest stream through which all Git events (repositories, commits, pull requests) flow between ingest services and downstream consumers.

## Architecture

- Embeds a **NATS JetStream server** that persists events to disk.
- Owns the **INGEST stream**, which captures all subjects under `ingest.>`.
- Exposes a **publisher API** (`GITPublisher`) that ingest services use to emit events.
- Runs **durable consumers** to process inbound events.

## Stream

| Attribute | Value |
|-----------|-------|
| Name | `INGEST` |
| Subject filter | `ingest.>` |
| Storage | File (JetStream persistent) |

## Subjects

| Subject | Payload | Description |
|---------|---------|-------------|
| `ingest.git.repo` | `model.Repo` JSON | A repository discovered by an ingestor |
| `ingest.git.commit` | `model.Commit` JSON | A commit discovered by an ingestor |
| `ingest.git.pullrequest` | `model.PullRequest` JSON | A pull request discovered by an ingestor |

## Consumers

### `ingest-git-repo`

| Attribute | Value |
|-----------|-------|
| Subject | `ingest.git.repo` |
| Durable | yes |
| Ack policy | Explicit |
| Max deliver | 5 |

On receipt: deserialises the JSON payload into `model.Repo` and acknowledges. If deserialisation fails, the message is negatively acknowledged and redelivered up to `MaxDeliver` times.

## Publisher API

The `GITPublisher` is the interface ingest services use to emit events into the domain. It connects to the NATS server and provides three methods:

| Method | Subject |
|--------|---------|
| `PublishRepo(ctx, model.Repo)` | `ingest.git.repo` |
| `PublishCommit(ctx, model.Commit)` | `ingest.git.commit` |
| `PublishPullRequest(ctx, model.PullRequest)` | `ingest.git.pullrequest` |

All payloads are serialised to JSON before publishing.

## Data Models

### `model.Repo`
| Field | Type | Description |
|-------|------|-------------|
| `RepoID` | string | Unique repository identifier |
| `Name` | string | Repository name |

### `model.Commit`
| Field | Type | Description |
|-------|------|-------------|
| `SHA` | string | Commit hash |
| `RepoID` | string | Repository the commit belongs to |
| `Author` | string | Commit author |
| `Timestamp` | time.Time | When the commit was made |

### `model.PullRequest`
| Field | Type | Description |
|-------|------|-------------|
| `ID` | string | Pull request identifier |
| `RepoID` | string | Repository the PR belongs to |
| `Name` | string | PR name |
| `Title` | string | PR title |
| `Description` | string | PR description |
| `State` | string | `OPEN`, `MERGED`, or `DECLINED` |
| `Author` | string | PR author |
| `CreatedAt` | time.Time | When the PR was opened |
| `UpdatedAt` | time.Time | When the PR was last updated |

## Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `--nats-host` | `0.0.0.0` | Address the NATS server binds on (persistent root flag) |
| `--nats-port` | `4222` | Port the NATS server listens on (persistent root flag) |
| `--nats-data` | `./data/nats` | Directory for JetStream persistence (persistent root flag) |
| `--nats-server` | `true` | Whether to start the embedded NATS server (persistent root flag) |

## Error Handling

- A NATS server that fails to start within the startup timeout aborts the process.
- A failed NATS client connection aborts startup.
- Messages that fail deserialisation are negatively acknowledged and redelivered; after `MaxDeliver` (5) attempts they are not redelivered further.

## Query API

The domain service exposes a typed query interface over core NATS request-reply. Any service (metrics generators, dashboards, etc.) can query domain entities without subscribing to the full event stream.

### Query subjects

| Subject | Entity | Description |
|---|---|---|
| `query.git.repo` | `model.Repo` | Query stored repositories |
| `query.git.commit` | `model.Commit` | Query stored commits |
| `query.git.pullrequest` | `model.PullRequest` | Query stored pull requests |
| `query.cicd.workflow` | `model.Workflow` | Query stored workflows |
| `query.cicd.workflowRun` | `model.WorkflowRun` | Query stored workflow runs |
| `query.cicd.workflowTask` | `model.WorkflowTask` | Query stored workflow tasks |

### Request format

Each request body is a JSON-encoded `query.Query`:

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

The server publishes N reply messages to the request's per-request inbox. Each message carries a `Daka-Query-Status` header:

| Header value | Body | Meaning |
|---|---|---|
| `data` | One JSON-encoded entity | A single matching result |
| `done` | Empty | Stream terminated cleanly |
| `error` | `{"error": "..."}` | Query failed; no further messages |

A client reads messages until it sees `done` or `error`. The `QueryClient` in `internal/domain/nats/query_client.go` implements this protocol and exposes per-entity streaming and collecting helpers.

### Go client

```go
client := dalinats.NewQueryClient(nc, logger)

// Streaming (non-blocking, good for large result sets):
out, errs := client.QueryCommits(ctx, query.Query{
    Entity: query.EntityCommit,
    Filter: &query.Filter{Op: query.OpTerm, Field: query.CommitRepoID, Value: &v},
})
for commit := range out { ... }
if err := <-errs; err != nil { ... }

// Collecting (convenience, loads all results):
commits, err := client.QueryCommitsAll(ctx, q)
```

## Future Work
