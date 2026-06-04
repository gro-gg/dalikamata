# Metrics Service — Requirements

## Purpose

Calculate engineering metrics from domain events and expose them to a Prometheus-compatible monitoring system.

## Architecture

- The metrics service queries the **domain service** via server-side aggregations.
- It exposes metrics via an **HTTP `/metrics` endpoint** (Prometheus pull model).

## Scope

Covers pull request cycle time (Bitbucket) and CI/CD workflow metrics (Jenkins).

## Metrics

### `pr_cycle_time_seconds` (Histogram)

Measures the time elapsed between a pull request being created and its current or final state.

| Attribute | Value |
|-----------|-------|
| Type | Histogram |
| Unit | Seconds |
| Buckets | 3600 (1h), 14400 (4h), 86400 (1d), 259200 (3d), 604800 (7d) |

**Labels:**

| Label | Description |
|-------|-------------|
| `repo_id` | Repository identifier (e.g. `PROJECT/repo-slug`) |
| `created_month` | Month the PR was opened, formatted as `YYYY-MM` |
| `state` | PR state: `OPEN`, `MERGED`, or `DECLINED` |

**Cycle time calculation per state:**

| State | Formula |
|-------|---------|
| `MERGED` | `updatedAt − createdAt` |
| `DECLINED` | `updatedAt − createdAt` |
| `OPEN` | `now − createdAt` (age at the time the event is received) |

**Update trigger:** computed on every Prometheus scrape via a server-side aggregation query to the domain.

### `workflow_run_duration_seconds` (Histogram)

Measures the total duration of a Jenkins pipeline run.

| Attribute | Value |
|-----------|-------|
| Type | Histogram |
| Unit | Seconds |
| Buckets | 60 (1m), 300 (5m), 900 (15m), 1800 (30m), 3600 (1h), 7200 (2h), 21600 (6h) |

**Labels:**

| Label | Description |
|-------|-------------|
| `team_name` | Team that owns the component; `unknown` if unclaimed |
| `component_name` | Component name from config YAML; `unknown` if unclaimed |
| `workflow_id` | Jenkins job name (e.g. `build-backend`) |
| `workflow_name` | Human-readable pipeline name |
| `status` | Build result: `SUCCESS`, `FAILURE`, or `ABORTED` |

`team_name` and `component_name` are resolved at query time from component YAML files ingested by `dalikamata ingest config`. Runs not claimed by any component file are labelled `unknown`.

**Update trigger:** computed on every Prometheus scrape via a server-side aggregation query to the domain.

### `workflow_task_duration_seconds` (Histogram)

Measures the duration of an individual pipeline stage within a Jenkins run.

| Attribute | Value |
|-----------|-------|
| Type | Histogram |
| Unit | Seconds |
| Buckets | 30, 60 (1m), 120 (2m), 300 (5m), 600 (10m), 1800 (30m), 3600 (1h) |

**Labels:**

| Label | Description |
|-------|-------------|
| `team_name` | Team that owns the component; `unknown` if unclaimed |
| `component_name` | Component name from config YAML; `unknown` if unclaimed |
| `workflow_id` | Jenkins job name |
| `workflow_name` | Human-readable pipeline name |
| `task_name` | Stage name (e.g. `Build`, `Test`, `Deploy`) |
| `status` | Stage result: `SUCCESS`, `FAILED`, or `ABORTED` |

**Update trigger:** computed on every Prometheus scrape via a server-side aggregation query to the domain.

## NATS Subjects

| Subject | Payload | Used by |
|---------|---------|---------|
| `ingest.git.pullrequest` | `model.PullRequest` JSON | `pr_cycle_time_seconds` |
| `ingest.cicd.workflowRun` | workflow run event | `workflow_run_duration_seconds` |
| `ingest.cicd.workflowTask` | workflow task/stage event | `workflow_task_duration_seconds` |

## HTTP Exposition

- Endpoint: `GET /metrics`
- Default listen address: `:2112`

## Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `--nats-url` | `localhost` | NATS server host name (persistent root flag) |
| `--nats-port` | `4222` | NATS server port (persistent root flag) |
| `--metrics-addr` | `:2112` | Metrics HTTP listen address |

## Error Handling

- A failed NATS connection aborts startup.
- Malformed or unparseable events are logged and skipped.

## Future Work