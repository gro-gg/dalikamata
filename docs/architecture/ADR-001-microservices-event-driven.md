# ADR-001: Microservice architecture with an event-driven backbone

## Status

Accepted

## Context

Continuous Delivery metrics vary significantly between organisations. The data sources that feed those metrics (version control systems, CI platforms, issue trackers) and the specific metrics that matter (cycle time, deployment frequency, change failure rate) differ by team, toolchain, and company culture. A monolithic system that hard-codes a fixed set of ingestors and metrics cannot serve this diversity without becoming a maintenance burden for contributors and a poor fit for adopters.

## Decision

Dalikamata is structured as a set of independent microservices communicating over an event-driven backbone (NATS JetStream).

The core services are:

- **Domain service** — owns the event stream and data model
- **Ingest services** — connect to external data sources and publish events
- **Metrics services** — subscribe to events and calculate metrics

## Rationale

An event-driven microservice architecture directly addresses the extensibility problem:

- **Custom ingestors** — organisations whose data source is not covered by a built-in ingest service can write their own microservice that publishes to the same NATS subjects. They do not need to fork or modify dalikamata itself.
- **Custom metrics** — organisations with company-specific metrics can write their own metrics service that subscribes to the event stream and exposes whatever Prometheus metrics they need, without touching the core codebase.
- **Independent deployment** — services can be deployed, scaled, and replaced independently. An organisation that does not need a particular ingestor simply does not run it.
- **Loose coupling** — publishers and subscribers have no direct dependency on each other. Adding a new ingestor automatically makes its events available to all existing and future metrics services.

## Consequences

- Each service has a well-defined responsibility and a narrow interface (NATS subjects and JSON payloads).
- The `mono` command is provided as a convenience to run all core services in a single process for simple deployments, without changing the underlying architecture.
- Contributors extending dalikamata must publish events using the established subject hierarchy (`ingest.git.*`) and data models (`pkg/model`) to remain compatible with existing consumers.
- Operational complexity is higher than a monolith; the `docker-compose-micro.yaml` and `docker-compose-mono.yaml` files are provided to lower the barrier to running the full system locally.
