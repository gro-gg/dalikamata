# dalikamata

Collect, generate and evaluate Continuous Delivery Metrics in your Organization.

Dalikamata (or Dalikmata) is a pre-colonial Visayan deity of the Philippines, revered as the goddess of eyes, eyesight, and clairvoyance. Known as a "many-eyed" deity, she is invoked to cure eye ailments, provide prophetic visions, and is believed to shed tears of dew at night over human suffering.

## Commands

| Command | Description |
|---|---|
| `dalikamata nats` | Start the embedded NATS JetStream server |
| `dalikamata domain` | Start the domain service — persists ingest events and serves typed queries (requires a running NATS server) |
| `dalikamata metrics` | Start the metrics service (requires a running NATS server) |
| `dalikamata ingest bitbucket` | Crawl Bitbucket and publish events to NATS |
| `dalikamata ingest jenkins` | Crawl Jenkins and publish pipeline events to NATS |
| `dalikamata ingest config` | Read component YAML files and publish platform events to NATS |
| `dalikamata mono` | Start NATS, domain, metrics, and ingest together in one process |

Key flags (available on all commands via root):

| Flag | Default | Description |
|---|---|---|
| `--nats-host` | `0.0.0.0` | NATS server hostname to connect to |
| `--nats-port` | `4222` | NATS server port |
| `--debug` | `false` | Enable debug logging |
| `--grace-period` | `10s` | Shutdown grace period |
| `--metrics-addr` | `0.0.0.0:2112` | Prometheus metrics listen address |

The `nats` and `mono` commands also accept `--nats-data` (default `./data/nats`) to set the JetStream persistence directory.

`dalikamata ingest config` flags:

| Flag | Default | Description |
|---|---|---|
| `--dir` | _(required)_ | Directory of per-component YAML files (`*.yaml` / `*.yml`) |

`dalikamata mono` also accepts `--components-dir` (optional) to run the config crawler alongside the other ingest sources.

## Docker Compose

Start all services individually (micro) — each service runs in its own container, with NATS in a dedicated container:

```bash
docker compose -f deploy/docker/docker-compose-micro.yaml up
```

Or start NATS, domain, ingest, and metrics together as a single process (mono):

```bash
docker compose -f deploy/docker/docker-compose-mono.yaml up
```

Add `--profile monitoring` to either command to also start Prometheus (port 9090) and Grafana (port 3000) for visualization. Omitting the flag runs the core services only — this is the mode used by e2e tests.

## Metrics

The metrics service (`dalikamata metrics`, port 2112 by default) exposes three Prometheus histograms on `/metrics`:

| Metric | Labels | Description |
|---|---|---|
| `pr_cycle_time_seconds` | `repo_id`, `created_month`, `state` | Time from PR creation to current or final state |
| `workflow_run_duration_seconds` | `team_name`, `component_name`, `workflow_id`, `workflow_name`, `status` | Total duration of a Jenkins pipeline run |
| `workflow_task_duration_seconds` | `team_name`, `component_name`, `workflow_id`, `workflow_name`, `task_name`, `status` | Duration of an individual pipeline stage |

`team_name` and `component_name` are resolved at query time from the component YAML files ingested by `dalikamata ingest config`. Workflows not claimed by any component file are labelled `team_name="unknown"` / `component_name="unknown"`.

Three Grafana dashboards are provisioned automatically when using the `--profile monitoring` Docker Compose flag:

| Dashboard | What it shows |
|---|---|
| **PR Cycle Time** | PR cycle-time percentiles by repository |
| **PR Performance Dashboard** | Average cycle time and PR count by repository |
| **Workflow Performance** | Run p50/p95/mean duration, total runs, slowest tasks by p95, and duration trends — filterable by team, component, and workflow |

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
