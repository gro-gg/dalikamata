# ADR-007: Per-Repo Self-Onboarding and Multi-Owner Workflow Resolution

**Status:** Accepted
**Date:** 2026-07-02 (self-onboarding); amended 2026-07-09 (multi-owner workflow resolution)
**Extends:** ADR-005 (component configuration as platform events)
**Amends:** ADR-006 (Domain Model Simplification) — see [Part 2](#part-2-multi-owner-workflow-resolution)

---

## Part 1: Per-Repo Self-Onboarding via In-Repo Config File

### Context

ADR-005 introduced Team and Component declarations as platform events, ingested from a central directory of YAML files (`dalikamata ingest config --dir <path>`). This central model requires a platform operator to maintain ownership mappings out-of-band from the repositories themselves.

Teams that own a repository are best placed to declare its ownership, and want that declaration to live *with the code* — reviewed, versioned, and moved alongside the repo. A central directory cannot be edited by repo owners without cross-team coordination, and drifts as repos are created, renamed, or archived.

### Decision

Allow a repository to **self-onboard** by committing a config file to its root. When the Bitbucket ingestor is started with `--component-config-enabled` (default `false`), it fetches the path given by `--component-config-file` (default `dalikamata.yaml`) from each crawled repository and, when present, publishes a single new `ingest.platform.repo` event per onboarded repo. That event carries the containing repo together with the declared `team` and `component`; the domain upserts the Team and the Component from it and records the repo's membership. Self-onboarding does **not** publish `ingest.platform.team` or `ingest.platform.component` events — that remains the central crawler's job, and the central crawler's behaviour (ADR-005) is unchanged.

**Schema** — the in-repo file omits the `repos:` list of the central schema, because the repository that contains the file *is* the implied sole member of the component:

```yaml
version: "1"
team: payments
component: payment-service
```

The `version` field is validated identically to ADR-005 (`"1"`); `team` and `component` are required. Parsing and validation reuse `internal/config/component` rather than duplicating the schema.

**Discovery** — an ordered list of candidate paths at the repo root (`--component-config-file`, comma-separated, default `dalikamata.yaml,dalikamata.yml,.dalikamata.yaml,.dalikamata.yml`), each fetched via the Bitbucket raw endpoint (`/rest/api/1.0/projects/{key}/repos/{slug}/raw/{path}`). The candidates are tried in order and the **first one present wins**; a `404` means "this candidate is absent" and is not an error. Bitbucket's raw API resolves one concrete path and has no glob/wildcard support, so the accepted extensions/locations are enumerated explicitly rather than matched by pattern. Cost is up to one request per candidate per repo, stopping at the first hit.

> Supporting true patterns (e.g. `*.yaml`) would require listing repo directories via the Bitbucket **browse** API (`/rest/api/1.0/projects/{key}/repos/{slug}/browse/{path}`) and a matching fake-server route. Deferred; the candidate list covers the `.yaml`/`.yml` and alternate-location cases without it.

**Event semantics** — the `ingest.platform.repo` event is *additive and reassigning*. Handling it *adds* the repo to the named component and — because a repo belongs to at most one component — *removes* that repo from every other component's membership. Re-publishing the same repo under a different component name therefore moves it; publishing several repos under the same component name merges them into one component.

**Fail-soft** — a missing, malformed, or unfetchable config file is logged and skipped. Self-onboarding must never abort a crawl: commit/PR ingestion for the repo continues regardless.

**Off by default** — without `--component-config-enabled`, the Bitbucket crawler never fetches file contents and behaves exactly as before. The platform publisher is only created when the flag is set.

### Consequences

- Multiple repos can self-onboard to the **same** `component` name and are merged into one component, so a component may legitimately span many repos across many self-onboarding files. Because a component has exactly one team, files that share a `component` name but declare **different** `team` names are a misconfiguration: the last event processed wins and sets the component's team. This is accepted for now and left to be detected/reported later.
- A repo changes ownership by being re-published under a different component name (it is stolen from the old one). This is the intended "move a repo to another component" flow.
- For each Component, either the central crawler (ADR-005) or self-onboarding is used, not both at the same time. Mixing the two for one component is possible but its behaviour is not defined.
- The Bitbucket crawler gains a dependency on `internal/config/component` (schema parsing/validation), reused from the central crawler rather than duplicated.
- Cost is up to one extra HTTP request per configured candidate path per repo per crawl when enabled (stopping at the first hit); when disabled there is no additional cost.
- The `INGEST` JetStream stream wildcard (`ingest.>`) already covers the new `ingest.platform.repo` subject — no stream reconfiguration is needed.
- The ingest path is idempotent (upsert by name / membership reassignment), so re-crawling after a config change is safe.

---

## Part 2: Multi-Owner Workflow Resolution

### Context

ADR-006 fixed Repo→Component as an n:1 relation: a repo belongs to at most one component. Separately, `Workflow.RepoIDs` is n:m — a Jenkins pipeline can check out several repos in one build (an application repo plus a shared library, most commonly — including one onboarded via Part 1 of this ADR). Combining an n:1 Repo→Component edge with an n:m Workflow→Repo edge means a **Workflow can legitimately have more than one owning component/team**: each of its repos resolves independently to (at most) one component, and those components can belong to different teams.

The ownership-resolution code introduced alongside the `RepoIDs` plurality change (commit `2c167d6`) did not reflect this: `newOwnerLookup` picked the **first** repo (in publish order) that mapped to a known component and used that single component/team as the workflow's owner, silently discarding any other owned repos. This was wrong — for example, a `build-backend` workflow checking out `PROJ/backend-api` (owned by `backend-team`) and `PROJ/shared-lib` (owned by `platform-team`) was attributed to `backend-team` alone, even when the shared library was itself owned. `GET /api/v1/workflowRuns?filter.team_name=platform-team` should return that workflow's runs too.

### Decision

A Workflow's owners are the **set** of (component, team) pairs reachable from **any** of its `RepoIDs`, not just the first that resolves. Concretely:

1. **Resolution**: for each repo in a workflow's `RepoIDs` (publish order), if it maps to a known component, that component (and its team, or `"unknown"` if the component has no team) is one of the workflow's owners. Repos that map to no component contribute nothing. Pairs are deduplicated preserving first-seen order. A workflow with no resolvable owner falls back to the single pair `("unknown", "unknown")`.
2. **Enriched fields become arrays under their existing JSON names**: `WorkflowRun.component_name`/`team_name` and `WorkflowTask.component_name`/`team_name` change from `string` to `[]string` — Elasticsearch-style multi-valued fields, not new field names, so `filter.team_name=<x>` continues to work unmodified at the URL level.
3. **List-field query semantics** (`internal/domain/query/evaluator.go`): `term`/`terms` filters on a `[]string` field match if **any** element matches; `exists` means non-empty; `range` and `sort` are not supported on list fields (return an error / are a stable no-op, respectively).
4. **`Workflow.repo_ids` becomes filterable again** (`filter.repo_ids=<repoID>`), reversing the "not filterable" restriction the scalar-only engine previously forced.
5. **Aggregation fan-out**: a terms aggregation on a `[]string` field (`internal/domain/query/aggregator.go`) buckets an item once per distinct element, so a doubly-owned run counts in each of its teams' buckets. Naively nesting independent `team_name`/`component_name` terms aggregations would produce the wrong cross-product pairing (e.g. attributing a `platform-team`-owned component to `backend-team`'s bucket). To keep pairs correlated, a projection-only pivot field is introduced — `RunOwner`/`TaskOwner`, one `"team|component"` string (`query.OwnerKey`) per owner pair — and the metrics service aggregates on that single field instead of nesting `by_team → by_component`, splitting the key back into the two Prometheus labels at emit time (`query.SplitOwnerKey`). Team names may not contain `|`.
6. **Ownership diagnostics become per-repo**: `OwnershipDiagnostics` (`/api/v1/ownership`) reports one `RepoOwnership{repo_id, component_name?, team_name?, reason}` entry per repo in the workflow's `RepoIDs`, plus a top-level `Reason` (`ok` if at least one repo resolves fully, `missing_repo_id`, `no_team_for_component`, or `no_component_for_repo` in that precedence), rather than a single flat resolution.

### Consequences

- **Breaking JSON shape**: any consumer parsing `component_name`/`team_name` on `WorkflowRun`/`WorkflowTask`, or the shape of `OwnershipDiagnostics`, as a scalar/flat object breaks. The NATS query port and its clients (API service, metrics service) share the same model JSON, so they must be deployed together — a mixed-version rollout where an old client decodes a new server's response (or vice versa) will fail to unmarshal.
- **Prometheus double-counting is intentional and documented**: `sum(workflow_run_duration_seconds)` across all `team_name` label values double-counts a run owned by two teams (it appears once per owning team's series). Per-team/per-component dashboard panels — the actual use case — are accurate; only an unfiltered global sum is affected.
- **`|` is reserved in team names** as the `OwnerKey` separator (`strings.Cut` on the first occurrence, so component names may safely contain `|`).
- Range filters and sorting on `team_name`/`component_name`/`repo_ids` now return an error instead of silently comparing/sorting a single string — these were not part of the documented API surface for the affected fields.
- ADR-006's Repo→Component n:1 decision is unchanged; this ADR only changes how Workflow ownership is *derived* from that relation when a workflow spans multiple repos.
