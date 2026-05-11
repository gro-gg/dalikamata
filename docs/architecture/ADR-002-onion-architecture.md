# ADR-002: Onion architecture for the domain service; pragmatic structure for ingestors

## Status

Accepted

## Context

As the system grows, the domain service will accumulate business logic around event processing and persistence. Without a clear structural principle, that logic risks becoming entangled with infrastructure concerns (NATS, databases, HTTP), making it hard to test, reason about, and extend.

Ingestors, by contrast, have a narrow and well-defined job: connect to an external data source, fetch data, and publish events. Imposing the same architectural rigour on services this simple adds ceremony without benefit.

## Decision

The **domain service** follows onion architecture with a ports-and-adapters (hexagonal) structure.

The **ingestors** live on the outermost layer of that architecture and do not need to follow onion architecture internally, provided they remain simple fetch-and-publish pipelines.

## Layers (domain service)

From innermost to outermost:

**1. Domain model** (`pkg/model`)
The core data structures — `Repo`, `Commit`, `PullRequest` — with no dependencies on any framework or infrastructure.

**2. Domain logic** (`internal/domain/`)
Business rules and port interfaces. Outgoing ports (e.g. a persistence repository interface) are defined here as Go interfaces. The domain logic depends only on the model and its own port interfaces — never on a concrete adapter.

**3. Application layer** (`internal/app/`)
Wires together domain logic and adapters. Orchestrates startup and injects concrete adapter implementations into the domain via its port interfaces. Contains no business logic of its own.

**4. Adapters / infrastructure** (outermost)
Concrete implementations of the domain's port interfaces:

- **Incoming ports** — adapters that receive external events and drive domain logic (e.g. `internal/domain/nats/port.go`, a NATS consumer)
- **Outgoing ports** — adapters that the domain drives outward, such as persistence adapters that implement repository interfaces defined in the domain layer

## Ingestors

Ingestors (`internal/ingest/`) sit on the outermost layer. They are infrastructure: they connect to an external API, transform the response into domain model objects, and publish them via the `GITPublisher` port.

An ingestor does not contain domain logic and does not need its own onion structure as long as it remains a simple pipeline:

```
External API → Client → Crawler → GITPublisher (outgoing port)
```

If an ingestor grows complex enough to have its own business rules, it should adopt the same layered structure at that point.

## Current code mapping

| Code | Onion layer | Role |
|------|-------------|------|
| `pkg/model/` | Domain model | Core entities — no external dependencies |
| `internal/domain/` | Domain logic | Business rules and port interfaces |
| `internal/domain/nats/port.go` | Adapter (incoming) | NATS consumer that drives domain logic |
| `internal/domain/nats/publisher.go` | Adapter (outgoing) | Publishes events to NATS; its interface belongs in the domain layer |
| `internal/app/` | Application layer | Wiring and dependency injection — no business logic |
| `internal/ingest/bitbucket/` | Outer layer | Fetch-and-publish pipeline — intentionally thin |
| `internal/metrics/` | Outer layer | Subscribes to events and exposes HTTP metrics |

## Key architectural rule

Port interfaces must be defined in the domain layer, not in the adapter layer. Adapters depend on the domain; the domain never depends on adapters.

The `GITPublisher` interface that ingestors depend on is currently defined alongside its NATS implementation. It should be extracted to the domain layer (`internal/domain/`) so that ingestors depend on the domain interface, not on the NATS adapter.

## Consequences

- Adding persistence means defining a repository interface (e.g. `RepoRepository`) in the domain layer and providing a concrete adapter (e.g. PostgreSQL) in the infrastructure layer. The domain never imports a database package.
- The application layer (`internal/app/domain.go`) is responsible for injecting the correct adapter at startup.
- Business logic discovered inside an ingestor is a signal it belongs in the domain layer instead.
- The `GITPublisher` interface should be moved to `internal/domain/` as a tracked follow-up to this decision.
