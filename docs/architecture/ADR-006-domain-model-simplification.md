# ADR-006: Domain Model Simplification — n:1 Repo→Component, No Role, No Component-Workflow Relation

**Status:** Accepted  
**Date:** 2026-06-23  
**Amends:** ADR-005 (Component Configuration as NATS-Ingested Platform Events)

---

## Context

ADR-005 introduced `Component` with two join entities — `ComponentRepo` and `ComponentWorkflow` — both carrying a `Role` attribute (`CI`/`CD`/`CICD`). After implementation, three design choices turned out to be over-engineered:

1. **n:m Repo↔Component** — The join table `ComponentRepo` was modelled to allow a single repo to belong to multiple components (monorepo support). No real use case for this exists today.
2. **Role on join entities** — `ComponentRepo.Role` and `ComponentWorkflow.Role` were added speculatively. No query or metric currently reads the role.
3. **Component→Workflow direct relation** — `ComponentWorkflow` links a component directly to workflows. This is redundant: a component's workflows are already reachable via its repos (`Component → Repos → Workflows`) if Repo↔Component is an n:1 relation.

---

## Decision

### 1. Repo→Component is n:1

A repo belongs to **at most one** component. The `ComponentRepo` join entity is removed. Instead, `Repo` carries an optional `ComponentName` field (empty string when unassigned). `Component` no longer owns a `Repos` list — its repos are discovered by querying repos where `ComponentName` matches.

Monorepo support (multiple components in one repo) is explicitly out of scope. If it becomes necessary it will be revisited as its own ADR.

### 2. Remove Role from all relations

The `Role` field is dropped from `ComponentRepo` and `ComponentWorkflow`. If role information is ever required, it will be added as a property on `Workflow` itself — not as a join attribute — keeping the model flat.

### 3. No Component→Workflow relation

`ComponentWorkflow` is removed entirely. A component's workflows are discovered by traversing `Component → Repos → Workflows`. No direct `Component`↔`Workflow` edge is maintained in the domain model.

---

## Consequences

- `ComponentRepo` and `ComponentWorkflow` structs and their associated NATS events are removed.
- `Repo` gains an optional `ComponentName string` field; the config crawler sets it when a component claims a repo.
- `Component` becomes a lightweight record: `Name` + `TeamName`. Its repos and workflows are query-time projections, not stored lists.
- The config YAML schema continues to declare repos under a component; the crawler resolves the n:1 ownership and publishes updated `Repo` events carrying the `ComponentName`.
- If two config files claim the same repo for different components, the last write wins (consistent with the existing upsert semantics from ADR-005). This is OK because the config will be moved to repo-level in the future.
- The updated ERD replaces the two join entities with a single optional foreign-key edge: `Repo }o--o| Component`.