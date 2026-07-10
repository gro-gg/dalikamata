# dalikamata

Collect, generate and evaluate Continuous Delivery Metrics in your Organization.

Dalikamata (or Dalikmata) is a pre-colonial Visayan deity of the Philippines, revered as the goddess of eyes, eyesight, and clairvoyance. Known as a "many-eyed" deity, she is invoked to cure eye ailments, provide prophetic visions, and is believed to shed tears of dew at night over human suffering.

## Commands

| Command | Description |
|---|---|
| `dalikamata nats` | Start the embedded NATS JetStream server |
| `dalikamata domain` | Start the domain service — persists ingest events and serves typed queries (requires a running NATS server) |
| `dalikamata metrics` | Start the metrics service (requires a running NATS server) |
| `dalikamata api` | Start the HTTP query API service (requires a running NATS server) |
| `dalikamata ingest bitbucket` | Crawl Bitbucket and publish events to NATS |
| `dalikamata ingest jenkins` | Crawl Jenkins and publish pipeline events to NATS |
| `dalikamata ingest config` | Read component YAML files and publish platform events to NATS |
| `dalikamata mono` | Start NATS, domain, metrics, API, and ingest together in one process |

Key flags (available on all commands via root):

| Flag | Default | Description |
|---|---|---|
| `--nats-host` | `0.0.0.0` | NATS server hostname to connect to |
| `--nats-port` | `4222` | NATS server port |
| `--debug` | `false` | Enable debug logging |
| `--grace-period` | `10s` | Shutdown grace period |
| `--ca-certs-dir` | _(none)_ | Directory containing custom CA certificates (`.pem`, `.crt`, `.cer`) |

`dalikamata nats` flags:

| Flag | Default | Description |
|---|---|---|
| `--nats-data` | `./data/nats` | Set the JetStream persistence directory |


`dalikamata domain` flags:

| Flag | Default | Description |
|---|---|---|
| `--db-path` | _(none)_ | Enables SQLite persistence for domain entities. When omitted, an in-memory repository is used and entity state is lost on restart. |

`dalikamata ingest bitbucket` flags:

| Flag | Default | Description |
|---|---|---|
| `--bitbucket-url` | _(required)_ | Bitbucket Server base URL (e.g. `https://bitbucket.example.com`) |
| `--bitbucket-token` | _(required)_ | Bitbucket personal access token |
| `--bitbucket-projects` | _(required)_ | Comma-separated list of Bitbucket project keys to crawl |
| `--bitbucket-interval` | `5m` | How often to re-crawl for new commits and pull requests |
| `--bitbucket-component-config-enabled` | `false` | Enable per-repo self-onboarding: fetch an in-repo config file from each repo root |
| `--bitbucket-component-config-file` | `dalikamata.yaml,...` | Candidate in-repo config paths tried per repo (comma-separated, first match wins; requires `--bitbucket-component-config-enabled`) |

The Bitbucket ingestor runs on a repeating ticker loop. The first crawl fires immediately on startup; subsequent crawls are spaced by `--bitbucket-interval`. Each repo's newest published commit SHA is persisted in a JetStream KV bucket (`ingest-bitbucket-cursors`) so that restarts do not re-ingest already-published commits. Only new commits (those reachable from the default branch tip but not from the cursor SHA) are fetched on subsequent ticks; pull requests and repos are refetched in full on every tick (they are small and re-publish is idempotent).

`dalikamata ingest jenkins` flags:

| Flag | Default | Description |
|---|---|---|
| `--jenkins-url` | _(required)_ | Jenkins base URL (e.g. `https://jenkins.example.com`) |
| `--jenkins-user` | _(required)_ | Jenkins username |
| `--jenkins-token` | _(required)_ | Jenkins API token |
| `--jenkins-jobs` | _(all jobs)_ | Comma-separated list of full job paths to crawl; discovers all if omitted |
| `--jenkins-repo-override` | _(none)_ | Comma-separated `pipeline=PROJ/repo` pairs; pin a repo ID when Jenkins BuildData is not populated (see [Team→workflow resolution](#teamworkflow-resolution)) |
| `--jenkins-interval` | `5m` | How often to re-crawl Jenkins for new builds |

The Jenkins ingestor runs on the same repeating ticker pattern as the Bitbucket ingestor. The first crawl fires immediately on startup; subsequent crawls are spaced by `--jenkins-interval`. Before fetching the full build list for a job, the crawler probes `lastCompletedBuild[number]` (a single-field Jenkins API call) to check whether anything has finished since the last tick — if the probe matches the stored cursor the full fetch is skipped entirely. Each job's highest published build number is persisted in a JetStream KV bucket (`ingest-jenkins-cursors`) so that restarts do not re-ingest already-published builds. Cursors are keyed by full job path (e.g. `shared-lib/main`) so branches of a multibranch pipeline track independent watermarks while still collapsing onto one `Workflow` entity in the domain.

`dalikamata ingest config` flags:

| Flag | Default | Description |
|---|---|---|
| `--component-config-dir` | _(required)_ | Directory of per-component YAML files (`*.yaml` / `*.yml`) |

`dalikamata metrics` flags:

| Flag | Default | Description |
|---|---|---|
| `--metrics-addr` | `0.0.0.0:2112` | Prometheus metrics listen address |
| `--metrics-refresh-interval` | `30s` | How often background loops recompute each metric |
| `--metrics-aggregate-timeout` | `30s` | Per-aggregation query timeout for metric refresh loops |

`dalikamata api` flags:

| Flag | Default | Description |
|---|---|---|
| `--api-addr` | `0.0.0.0:2113` | HTTP query API listen address |
| `--api-query-timeout` | `30s` | Per-request query timeout for the API server |

`dalikamata mono` flags:

The `mono` command is a convenience wrapper that starts NATS, domain, metrics, API, and all three ingestors in a single process. It accepts all flags from the individual commands.

## Docker Compose

Start all services individually (micro) — each service runs in its own container, with NATS in a dedicated container:

```bash
docker compose -f deploy/docker/docker-compose-micro.yaml up
```

Add SQLite persistence for the domain service (entities survive restarts):

```bash
docker compose -f deploy/docker/docker-compose-micro.yaml \
               -f deploy/docker/docker-compose-micro-persist.yaml up
```

Or start NATS, domain, ingest, and metrics together as a single process (mono):

```bash
docker compose -f deploy/docker/docker-compose-mono.yaml up
```

Add `--profile monitoring` to either command to also start Prometheus (port 9090) and Grafana (port 3000) for visualization. Omitting the flag runs the core services only — this is the mode used by e2e tests.

## Metrics

The metrics service (`dalikamata metrics`, port 2112 by default) exposes three Prometheus histograms on `/metrics`. Each metric is computed by its own background goroutine loop; Prometheus scrapes are served from the last cached values and never block on live aggregation queries. The cache is updated every `--metrics-refresh-interval` (default `30s`).

| Metric | Labels | Description |
|---|---|---|
| `pr_cycle_time_seconds` | `repo_id`, `created_month`, `state` | Time from PR creation to current or final state |
| `workflow_run_duration_seconds` | `team_name`, `component_name`, `workflow_id`, `workflow_name`, `branch`, `status` | Total duration of a Jenkins pipeline run |
| `workflow_task_duration_seconds` | `team_name`, `component_name`, `workflow_id`, `workflow_name`, `branch`, `task_name`, `task_order`, `status` | Duration of an individual pipeline stage |

`team_name` and `component_name` are resolved at query time from the component YAML files ingested by `dalikamata ingest config`. The resolution chain is:

```
Workflow.RepoIDs  →  Component.RepoIDs (contains)  →  Component.TeamName
```

A Jenkins build often checks out several repos (the application repo plus shared libraries), so a `Workflow` carries **all** of them, and resolves to **all** of the components/teams they belong to — not just the first. `team_name`/`component_name` are therefore lists (e.g. `workflow_run_duration_seconds{team_name="platform",...}` and `{team_name="backend-team",...}` can both carry observations for the same run), and a doubly-owned run is counted once per owning team. A repo that no component claims is skipped; a workflow that cannot be resolved through any of its repos is labelled `team_name="unknown"` / `component_name="unknown"`. Use the [ownership diagnostics endpoint](#ownership-diagnostics) to find out which repo fails and why.

Six Grafana dashboards are provisioned automatically when using the `--profile monitoring` Docker Compose flag:

| Dashboard | What it shows |
|---|---|
| **PR Cycle Time** | PR cycle-time percentiles by repository |
| **PR Performance Dashboard** | Average cycle time and PR count by repository |
| **Workflow Performance** | Run p50/p95/mean duration, total runs, slowest tasks by p95, and duration trends — filterable by team, component, and workflow |
| **Last 10 Workflow Builds** | Status table of the last 10 runs and their task breakdown for a selected team and workflow |
| **Workflow Task Breakdown** | Mean task duration per pipeline stage for a selected team and workflow |
| **Team Workflow Overview** | Recent build status timeline per workflow, successful-run and successful-task duration trends, and mean task duration by stage — all scoped to a single team |

## Query API

The API service (`dalikamata api`, port 2113 by default) exposes the domain's
accumulated state as a JSON HTTP API, intended for use with Grafana's
[Infinity data source](https://grafana.com/grafana/plugins/yesoreyeram-infinity-datasource/)
to build dashboards showing raw entity data alongside Prometheus metrics.

Eight entity endpoints support both GET (URL params) and POST
(full `query.Query` JSON body):

```
GET/POST /api/v1/repos
GET/POST /api/v1/commits
GET/POST /api/v1/pullrequests
GET/POST /api/v1/workflows
GET/POST /api/v1/workflowRuns
GET/POST /api/v1/workflowTasks
GET/POST /api/v1/teams
GET/POST /api/v1/components
```

### Ownership diagnostics

```
GET /api/v1/ownership
```

Returns a JSON array with one entry per known `Workflow`. Because a workflow can reference several repos belonging to different components, there is no single resolution outcome — `owners` reports the per-repo breakdown through the `Workflow.RepoIDs → Component.RepoIDs → Component.TeamName` chain, and the top-level `reason` is `"ok"` as soon as at least one repo resolves fully:

```json
[
  { "workflow_id": "payments/build", "reason": "ok", "owners": [
      { "repo_id": "PROJ/payments", "component_name": "payments-svc", "team_name": "payments", "reason": "ok" },
      { "repo_id": "PROJ/shared-lib", "reason": "no_component_for_repo" }
  ] },
  { "workflow_id": "old-job", "reason": "missing_repo_id" }
]
```

| `reason` | Meaning | Fix |
|---|---|---|
| `ok` | At least one repo resolved fully | — |
| `missing_repo_id` | `Workflow.RepoIDs` is empty — Jenkins BuildData not populated | Add `--jenkins-repo-override old-job=PROJ/my-repo` |
| `no_component_for_repo` | No component YAML lists any of the workflow's repos | Add the repo ID to the matching component's `repo_ids` list |
| `no_team_for_component` | At least one repo reached a component, but none reached a team | Set `team:` in the component YAML file |

Common GET parameters:

| Parameter | Example | Meaning |
|---|---|---|
| `size` | `size=10` | Max hits (default 100; `0` = all) |
| `from` | `from=20` | Offset for pagination |
| `sort` | `sort=-started_at,task_order` | Comma-separated; `-` prefix = descending |
| `filter.<field>` | `filter.team_name=platform` | Term filter; repeat key for multi-value |
| `filter.<field>.gte/.lte/.gt/.lt` | `filter.started_at.gte=2026-01-01T00:00:00Z` | Range filter |
| `filter.<field>.exists` | `filter.commit_sha.exists=true` | Exists filter |

Field names are the constants in `internal/domain/query/fields.go`. All
`WorkflowRun` and `WorkflowTask` responses include `team_name`,
`component_name`, and `workflow_name` enriched from the component YAML
configuration — no joins needed in dashboards.

**Example: last 10 workflow runs for a team with their tasks**

```
# Panel 1 — runs (drive $runIds variable from column "id")
GET /api/v1/workflowRuns?filter.team_name=$team&sort=-started_at&size=10

# Panel 2 — tasks for those runs
GET /api/v1/workflowTasks?filter.workflow_run_id=$runIds&sort=workflow_run_id,task_order
```

For complex queries (nested bool filters, server-side aggregations), use POST
with a `query.Query` JSON body. See `docs/architecture/api.md` for the full
design and a list of planned future enhancements.

# Testing

### Unit tests

```bash
go test ./... -race
```

### End-to-end tests

E2E tests spin up the full stack using Docker Compose and verify that metrics
are produced and NATS messages are flowing. They require Docker to be running
and are gated behind the `e2e` build tag.

```bash
go test -tags=e2e ./internal/e2e/... -v -timeout 2m
```

By default, the test suite builds the Docker images before running. If the images
are already loaded (e.g. in CI after a dedicated build step), pass
`-skip-docker-build` to skip the build:

```bash
go test -tags=e2e ./internal/e2e/... -v -timeout 2m -skip-docker-build
```
