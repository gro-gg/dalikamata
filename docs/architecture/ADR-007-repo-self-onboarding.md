# ADR-007: Per-Repo Self-Onboarding via In-Repo Config File

**Status:** Accepted  
**Date:** 2026-07-02  
**Extends:** ADR-005 (component configuration as platform events)

---

## Context

ADR-005 introduced Team and Component declarations as platform events, ingested from a central directory of YAML files (`dalikamata ingest config --dir <path>`). This central model requires a platform operator to maintain ownership mappings out-of-band from the repositories themselves.

Teams that own a repository are best placed to declare its ownership, and want that declaration to live *with the code* — reviewed, versioned, and moved alongside the repo. A central directory cannot be edited by repo owners without cross-team coordination, and drifts as repos are created, renamed, or archived.

---

## Decision

Allow a repository to **self-onboard** by committing a config file to its root. When the Bitbucket ingestor is started with `--component-config-enabled` (default `false`), it fetches the path given by `--component-config-file` (default `dalikamata.yaml`) from each crawled repository and, when present, publishes a single new `ingest.platform.repo` event per onboarded repo. That event carries the containing repo together with the declared `team` and `component`; the domain upserts the Team and the Component from it and records the repo's membership. Self-onboarding does **not** publish `ingest.platform.team` or `ingest.platform.component` events — that remains the central crawler's job, and the central crawler's behaviour (ADR-005) is unchanged.

**Schema** — the in-repo file omits the `repos:` list of the central schema, because the repository that contains the file *is* the implied sole member of the component:

```yaml
version: "1"
team: payments
component: payment-service
```

The `version` field is validated identically to ADR-005 (`"1"`); `team` and `component` are required. Parsing and validation reuse `internal/config/component` rather than duplicating the schema.

**Discovery** — a configurable path at the repo root (`--component-config-file`, default `dalikamata.yaml`), fetched via the Bitbucket raw endpoint (`/rest/api/1.0/projects/{key}/repos/{slug}/raw/{path}`), one request per repo. A `404` means "not onboarded" and is not an error.

**Event semantics** — the `ingest.platform.repo` event is *additive and reassigning*. Handling it *adds* the repo to the named component and — because a repo belongs to at most one component — *removes* that repo from every other component's membership. Re-publishing the same repo under a different component name therefore moves it; publishing several repos under the same component name merges them into one component.

**Fail-soft** — a missing, malformed, or unfetchable config file is logged and skipped. Self-onboarding must never abort a crawl: commit/PR ingestion for the repo continues regardless.

**Off by default** — without `--component-config-enabled`, the Bitbucket crawler never fetches file contents and behaves exactly as before. The platform publisher is only created when the flag is set.

---

## Consequences

- Multiple repos can self-onboard to the **same** `component` name and are merged into one component, so a component may legitimately span many repos across many self-onboarding files. Because a component has exactly one team, files that share a `component` name but declare **different** `team` names are a misconfiguration: the last event processed wins and sets the component's team. This is accepted for now and left to be detected/reported later.
- A repo changes ownership by being re-published under a different component name (it is stolen from the old one). This is the intended "move a repo to another component" flow.
- For each Component, either the central crawler (ADR-005) or self-onboarding is used, not both at the same time. Mixing the two for one component is possible but its behaviour is not defined.
- The Bitbucket crawler gains a dependency on `internal/config/component` (schema parsing/validation), reused from the central crawler rather than duplicated.
- Cost is one extra HTTP request per repo per crawl when enabled; when disabled there is no additional cost.
- The `INGEST` JetStream stream wildcard (`ingest.>`) already covers the new `ingest.platform.repo` subject — no stream reconfiguration is needed.
- The ingest path is idempotent (upsert by name / membership reassignment), so re-crawling after a config change is safe.
