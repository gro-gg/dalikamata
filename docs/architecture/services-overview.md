# Services Overview

This diagram shows all microservices, their communication channels, and how metrics are produced.

```mermaid
graph TB
    subgraph sources["External Data Sources"]
        BB["Bitbucket Server\n(HTTP REST API)"]
        JK["Jenkins Server\n(HTTP REST API)"]
        CF["Component Config\n(YAML files, filesystem)"]
    end

    subgraph ingest["Ingest Services"]
        IB["ingest bitbucket\n─ crawls repos, commits, PRs\n─ persists cursors in KV bucket"]
        IJ["ingest jenkins\n─ crawls jobs, builds, stages"]
        IC["ingest config\n─ reads component YAML"]
        CX["custom ingest\n(user-provided)"]
    end

    subgraph nats["NATS JetStream Server  ·  :4222"]
        IS[("INGEST stream\n──────────────────────\ningest.git.repo\ningest.git.commit\ningest.git.pullrequest\ningest.cicd.workflow\ningest.cicd.workflowRun\ningest.cicd.workflowTask\ningest.platform.team\ningest.platform.component")]
        QCH[("Query subjects  ·  request-reply\n──────────────────────────────\nquery.git.*\nquery.cicd.*\nquery.platform.*\nquery.aggregate")]
    end

    subgraph domain["Domain Service  ·  dalikamata domain"]
        NP["NATSPort\ndurable JetStream consumer"]
        DS["DomainService"]
        QP["QueryPort\nrequest-reply responder"]
        REPO[("In-Memory\nRepository")]
    end

    subgraph consumers["Consumer Services"]
        MS["Metrics Service\ndalikamata metrics  ·  :2112 /metrics"]
        AS["API Service\ndalikamata api  ·  :2113 /api/v1/*"]
    end

    subgraph observers["Observers / Dashboards"]
        PROM["Prometheus"]
        GRAF["Grafana"]
    end

    BB -->|"HTTP REST"| IB
    JK -->|"HTTP REST"| IJ
    CF -->|"filesystem read"| IC

    IB -->|"ingest.git.*\nJetStream publish"| IS
    IJ -->|"ingest.cicd.*\nJetStream publish"| IS
    IC -->|"ingest.platform.*\nJetStream publish"| IS
    CX -->|"ingest.*\nJetStream publish"| IS

    IS -->|"durable consumer"| NP
    NP --> DS
    DS <--> REPO
    DS --> QP
    QP <-->|"request-reply"| QCH

    MS -->|"QueryClient\nquery.aggregate"| QCH
    AS -->|"QueryClient\nquery.* + query.aggregate"| QCH

    PROM -->|"HTTP scrape /metrics"| MS
    GRAF -->|"PromQL"| PROM
    GRAF -->|"HTTP GET/POST /api/v1/*"| AS
```

## Communication protocols

| Channel | Protocol | Direction |
|---|---|---|
| Ingest crawlers → NATS | NATS JetStream publish | crawlers push events to the `INGEST` durable stream |
| NATS → Domain (`NATSPort`) | JetStream durable consumer | domain pulls from the stream at its own pace |
| Domain (`QueryPort`) ↔ NATS | Core NATS request-reply | QueryPort subscribes to `query.*` and `query.aggregate`; callers use `QueryClient` to send requests and receive streamed replies |
| Metrics / API → Domain | Core NATS request-reply (via `QueryClient`) | services send a `query.Query` JSON body; domain streams back `data` messages then a `done` sentinel, each with a `Daka-Query-Status` header |
| Prometheus → Metrics | HTTP scrape `/metrics` | Prometheus polls on its configured interval; responses are served from a pre-computed cache updated every `--metric-refresh-interval` |
| Grafana → API | HTTP GET / POST `/api/v1/*` | Grafana Infinity datasource drives live queries; supports filter, sort, pagination, and enriched `team_name` / `component_name` labels |
| Grafana → Prometheus | PromQL | standard Prometheus datasource for histogram dashboards |

## Extension points

**Custom ingest sources** — any service that publishes to the established subject hierarchy (`ingest.git.*`, `ingest.cicd.*`, `ingest.platform.*`) using the `pkg/model` types is immediately consumed by the domain without any changes to the core codebase (see [ADR-001](ADR-001-microservices-event-driven.md)).

**Custom metrics services** — any service that connects to NATS and issues queries via `QueryClient` can read the accumulated domain state and expose its own Prometheus metrics or HTTP endpoints.

**Custom storage backends** — implementing `domain.Repository` and `domain.QueryRepository` in a new adapter (e.g. SQLite, PostgreSQL) and wiring it in `internal/app/domain.go` is the only change required; the NATS ports and all crawlers are unaffected (see [ADR-002](ADR-002-onion-architecture.md)).
