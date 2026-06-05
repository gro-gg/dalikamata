# API service — HTTP query adapter

## Overview

The `api` service (`dalikamata api`, port 2113 by default) is a thin HTTP adapter
over the existing NATS query layer. It lets Grafana's
[Infinity data source](https://grafana.com/grafana/plugins/yesoreyeram-infinity-datasource/)
consume raw domain objects alongside the Prometheus aggregated metrics already
served by the `metrics` service.

```
Grafana panel  ──(HTTP)──▶  dalikamata api (:2113)
 Infinity ds                internal/api.Server
                            ↓ NATS request-reply
                            internal/domain/nats.QueryPort
                            ↓
                            DomainService → MemoryRepository
```

The service introduces **no new NATS subjects** and **no domain or repo changes**.
It is a new inbound HTTP port that depends on the same `QueryFetcher` interface
(`*dalinats.QueryClient`) the metrics service already uses for its `Aggregate`
calls.

## API surface

### Endpoints

| Path | Entity |
| --- | --- |
| `GET/POST /api/v1/repos` | `model.Repo` |
| `GET/POST /api/v1/commits` | `model.Commit` |
| `GET/POST /api/v1/pullrequests` | `model.PullRequest` |
| `GET/POST /api/v1/workflows` | `model.Workflow` |
| `GET/POST /api/v1/workflowRuns` | `model.WorkflowRun` |
| `GET/POST /api/v1/workflowTasks` | `model.WorkflowTask` |
| `GET/POST /api/v1/teams` | `model.Team` |
| `GET/POST /api/v1/components` | `model.Component` |

All responses include enriched fields from the domain projection layer
(`team_name`, `component_name`, `workflow_name`) without any join code in the
API layer.

### GET — URL query parameters

| Parameter | Example | Meaning |
| --- | --- | --- |
| `size` | `size=10` | Max hits (default 100; `0` = all; `-1` = aggregations only) |
| `from` | `from=20` | Offset |
| `sort` | `sort=-started_at,task_order` | Comma-separated; `-` prefix = descending |
| `filter.<field>` | `filter.team_name=platform` | Term. Repeat key for multi-value: `filter.workflow_run_id=a&filter.workflow_run_id=b` |
| `filter.<field>.gte/.lte/.gt/.lt` | `filter.started_at.gte=2026-01-01T00:00:00Z` | Range (RFC3339 for time) |
| `filter.<field>.exists` | `filter.commit_sha.exists=true` | Exists |

Field names are the constants in `internal/domain/query/fields.go`. Unknown
field names return `400`.

### POST — full DSL passthrough

`POST /api/v1/{entity}` accepts a raw `query.Query` JSON body for filters the
URL-param syntax cannot express (nested `bool` / `must` / `should`, and
server-side `aggs`). When `size == -1` the response uses the aggregation shape
instead of the hits shape:

```json
{ "entity": "workflowRun", "aggregations": { ... } }
```

### Typical Grafana dashboard recipe

Two Infinity panels for "last 10 workflow runs and their tasks for team
`$team`":

```
Query A: GET /api/v1/workflowRuns?filter.team_name=$team&sort=-started_at&size=10
  → Table panel; drive a Grafana variable $runIds from column "id"

Query B: GET /api/v1/workflowTasks?filter.workflow_run_id=$runIds&sort=workflow_run_id,task_order
  → Table panel; grouped by workflow_run_id
```

Mean-time panels remain on the Prometheus data source — nothing changes there.

---

## Future work (out of scope for v1)

The following improvements are explicitly deferred and should be implemented
as the API surface evolves.

### 1. Authentication and CORS

The API currently has no authentication and does not set CORS headers. This is
acceptable while the service runs inside the cluster and Grafana proxies
requests server-side. Add a middleware layer (e.g. Bearer token validation,
mTLS, or API-key header) before exposing the port outside cluster boundaries.

CORS headers (`Access-Control-Allow-Origin`, preflight OPTIONS handling) are
needed if Grafana is ever configured to make browser-side API calls, or if
another client (e.g. a custom frontend) consumes the API from a different
origin.

**Suggested path:** a standard `net/http` middleware function that wraps the
mux returned by `Server.newMux()`, keeping auth logic cleanly separated from
handler logic. Gate it with a flag or env var so it can be disabled in
test/dev environments.

### 2. Server-Sent Events (SSE) for large result sets

The API currently buffers all matching entities before writing the response.
This is fine for dashboard-scale result sets (`size` ≤ a few thousand), but
will eventually hit memory limits for "give me everything" queries across
large repositories.

Add a streaming path behind a flag or a separate path prefix
(`/api/v1/stream/{entity}`) that pipes the `QueryClient` streaming channel
directly to an SSE response (`Content-Type: text/event-stream`). The
`QueryClient.QueryXRuns` (streaming) channel API already exists; no domain
changes are needed.

### 3. Custom Grafana data source plugin

The Infinity data source handles the common cases but requires dashboard authors
to know the URL param syntax and JSON field names. A custom Grafana plugin could
provide:
- A query builder UI for filter, sort, and pagination
- Autocomplete on team/component/workflow names (fed by the API itself)
- Native variable support without manual `$runIds` template expressions
- Typed field metadata for smarter rendering (e.g., duration columns formatted
  as human-readable times)

**Suggested path:** follow the [Grafana plugin SDK for Go or TypeScript](https://grafana.com/developers/plugin-tools);
the backend plugin wraps the same HTTP API endpoints the Infinity integration
already uses.

### 4. SQL-backed storage and Grafana SQL data source

The domain currently uses an in-memory `MemoryRepository` which is rebuilt from
NATS JetStream replay on startup. A SQL-backed `Repository` (e.g. SQLite or
PostgreSQL) would allow:
- Persistent storage across restarts without full event replay
- Grafana panels driven by SQL data source (ad-hoc SQL, no API layer required)
- Richer ad-hoc analysis without changing the Go query DSL

**Suggested path:** implement `domain.Repository` and `domain.QueryRepository`
in `internal/domain/repo/sqlite.go` (or `postgres.go`), swap it in
`internal/app/domain.go`, and add a migration tool. The domain layer and all
existing NATS adapters require no changes. See ADR-002 for the port contract.

### 5. Inline-embedded related entities (`?include=tasks`)

Dashboard authors currently need two separate queries to show workflow runs
with their tasks (Query A → runs, Query B → tasks filtered by `runIds`). An
`?include=tasks` parameter on `/api/v1/workflowRuns` that embeds task arrays
inline would simplify single-panel drill-through views.

Add only if dashboards repeatedly need the fan-out; the two-query Grafana
variable pattern handles the v1 case without any backend change.
