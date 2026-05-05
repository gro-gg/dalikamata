# Metrics Service — Requirements

## Purpose

Calculate engineering metrics from domain events and expose them to a Prometheus-compatible monitoring system.

## Architecture

- The metrics service subscribes to **NATS events** published by ingest services.
- It exposes metrics via an **HTTP `/metrics` endpoint** (Prometheus pull model).
- In a future iteration, it will additionally query the domain service directly to calculate metrics that require historical or aggregated data.

## Scope (v1)

The first version subscribes only to pull request events from the Bitbucket ingestor and calculates a single metric: **PR cycle time**.

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

**Update trigger:** one histogram observation is recorded for each incoming NATS pull request event.

## NATS Subscription

| Subject | Payload |
|---------|---------|
| `ingest.git.pullrequest` | `model.PullRequest` JSON |

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