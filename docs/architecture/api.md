# API service — HTTP query adapter

## Overview

The `api` service (`dalikamata api`, port 2113 by default) is a thin HTTP adapter
over the existing NATS query layer. It lets Grafana's
[Infinity data source](https://grafana.com/grafana/plugins/yesoreyeram-infinity-datasource/)
consume raw domain objects alongside the Prometheus aggregated metrics already
served by the `metrics` service.

The authoritative API specification is `api/openapi.yaml` (OpenAPI 3.1). The
`api` command serves it at `/` alongside a Scalar API reference UI.

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

## Known gaps (out of scope for v1)

Auth/CORS middleware, SSE streaming for large result sets, a custom Grafana data source plugin, and `?include=tasks` inline embedding are tracked as follow-ups.
