# ADR-005: Component Configuration as NATS-Ingested Platform Events

**Status:** Accepted  
**Date:** 2026-05-29  
**Extends:** ADR-001 (event-driven backbone), ADR-002 (onion architecture), ADR-003 (query DSL)

---

## Context

Dalikamata ingests *observed* events (commits, PRs, workflow runs) from source systems, but had no concept of **what is being delivered** — which repos and pipelines belong to which deployable unit, and which team owns it. Without this context it is impossible to compute per-component or per-team CD metrics.

---

## Decision

Introduce per-component YAML configuration files. Each file declares a Component with a name, an owning Team, a list of Repos (tagged CI/CD/CICD), and a list of Workflows (tagged CI/CD/CICD).

**Ingest path** — files are read by a new one-shot crawler (`dalikamata ingest config --component-config-dir <path>`) that publishes `ingest.platform.team` and `ingest.platform.component` events to the existing `INGEST` JetStream stream. The domain service persists them through the same event/query bus as every other entity (ADR-001 conformant).

**Role model** — CI/CD/CICD is a *join attribute* on `ComponentRepo` / `ComponentWorkflow`, not a field on `model.Repo` / `model.Workflow`. A repo can play different roles in different components.

**Query exposure** — `model.Team` and `model.Component` are first-class query entities reachable at `query.platform.team` and `query.platform.component`, consistent with ADR-003.

**Schema versioning** — YAML files carry a `version` field (currently `"1"`). Unknown versions are rejected at load time so future schema evolution is backward-safe.

---

## Consequences

- Component and Team entities are queryable alongside Repo/Workflow/etc., enabling future per-component and per-team metric computation.
- The config crawler is idempotent: the domain repository upserts by name, so re-running after a YAML change is safe.
- A repo or workflow is not required to appear in any component config; the ingest sources remain independent.
- The `INGEST` JetStream stream wildcard (`ingest.>`) already covers the new `ingest.platform.*` subjects — no stream reconfiguration is needed.
