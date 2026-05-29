# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build ./...

# Vet
go vet ./...

# Lint
golangci-lint run

# Unit tests (with race detector)
go test -race ./...

# Run a single test
go test -race ./internal/ingest/bitbucket/... -run TestFoo

# Integration tests (slow, start real subprocesses)
go test -tags=integration ./internal/ingest/bitbucket/... -v -timeout 20s
```

## Architecture

Dalikamata is a **Continuous Delivery metrics platform** built as event-driven microservices over **NATS JetStream**. Services publish/subscribe to a single durable stream (`INGEST`) using a subject hierarchy under `ingest.>`.

### Onion / ports-and-adapters layering

```
pkg/model/          Shared value objects (Repo, Commit, PullRequest, Job, Build, PipelineStage)
internal/domain/    Core domain — DomainService, port interfaces (GitPublisher, PipelinePublisher,
                    GitEventHandler, PipelineEventHandler, Repository)
internal/domain/nats/  NATS adapters: NATSPort (inbound — consumes stream, calls domain handlers),
                       GITPublisher / PipelinePublisher (outbound — publishes from crawlers)
internal/ingest/    Crawlers for external data sources (bitbucket, jenkins). Each crawler depends
                    on a domain.GitPublisher or domain.PipelinePublisher interface.
internal/metrics/   MetricsService — subscribes to PullRequest events, exposes Prometheus metrics
internal/nats/      Embedded NATS server (server.go) and retry-connect helper (connect.go)
internal/app/       Wiring layer — App structs read config fields and construct the service graph;
                    one App per command (NATSApp, DomainApp, MetricsApp, IngestBitbucketApp, …)
cmd/dalikamata/     Cobra commands that populate App structs from CLI flags and call Run
cmd/dalifakes/      Fake server binary used in tests (currently: BitbucketServer)
```

### NATS subjects

**Ingest (JetStream, durable consumers):**

| Subject | Payload type |
|---|---|
| `ingest.git.repo` | `model.Repo` |
| `ingest.git.commit` | `model.Commit` |
| `ingest.git.pullrequest` | `model.PullRequest` |
| `ingest.cicd.workflow` | `model.Workflow` |
| `ingest.cicd.workflowRun` | `model.WorkflowRun` |
| `ingest.cicd.workflowTask` | `model.WorkflowTask` |
| `ingest.platform.team` | `model.Team` |
| `ingest.platform.component` | `model.Component` |

All ingest subjects feed into the `INGEST` durable stream. Consumers are created by `internal/domain/nats/port.go`.

**Query (core NATS request-reply):**

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

Request body: JSON `query.Query`. Reply: N `data` messages + one `done` sentinel, each with a `Daka-Query-Status` header. See `internal/domain/nats/subjects.go` for constants, `internal/domain/query/` for the DSL types, and ADR-003 for the design rationale.

### Services and commands

| Command | App struct | What it runs |
|---|---|---|
| `dalikamata nats` | `NATSApp` | Embedded NATS JetStream server |
| `dalikamata domain` | `DomainApp` | Ingest `NATSPort` + `QueryPort` + `DomainService` (persists events; serves queries) |
| `dalikamata metrics` | `MetricsApp` | NATS subscriber → Prometheus `/metrics` HTTP server |
| `dalikamata ingest bitbucket` | `IngestBitbucketApp` | Bitbucket crawler → publishes git events |
| `dalikamata ingest config` | `IngestConfigApp` | Component YAML crawler → publishes platform events |
| `dalikamata ingest jenkins` | `IngestJenkinsApp` | Jenkins crawler → publishes pipeline events |
| `dalikamata mono` | — | Runs all of the above in one process |

### Fake servers

`internal/ingest/bitbucket/fakeserver` is a real Go HTTP server (not a test double) that implements the Bitbucket Server REST v1 API with hard-coded fixture data (2 projects, 5 repos, commits, PRs). It is used both by the `dalifakes bitbucket` CLI and by integration tests. Jenkins does not yet have a fakeserver equivalent.

### Data flow (Bitbucket ingest path)

```
dalifakes bitbucket  (HTTP server)
        ↓  HTTP
bitbucket.Crawler → domain.GitPublisher (NATS) → INGEST stream
        ↓  NATS consumer
domain.NATSPort → DomainService → Repository (in-memory)
        ↓  NATS consumer (separate)
metrics.NATSPort → MetricsService → Prometheus
```

### Key extension points

To add a new ingest source: implement `domain.GitPublisher` or `domain.CICDPublisher` and publish to the established subjects. No changes to the domain or metrics services are required.

To add a new metric: either subscribe to the `INGEST` stream (event-driven, like `internal/metrics`) or use the query API (`QueryClient` in `internal/domain/nats/query_client.go`) to read accumulated state from the domain. The query API supports filter, sort, and pagination over all six entity types; see `internal/domain/query/` for DSL types and ADR-003 for design details.

To add a new storage backend: implement `domain.Repository` and `domain.QueryRepository` in a new adapter (e.g. `internal/domain/repo/sqlite.go`) and swap it in `internal/app/domain.go`. The domain layer and NATS adapters require no changes.
