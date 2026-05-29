# Component Configuration Ingest

## Overview

The `ingest config` crawler reads per-component YAML files from a directory and
publishes `Team` and `Component` platform events to the NATS backbone. These
entities describe the ownership and delivery structure of software components —
which repos and pipelines belong to a component, and which team owns it.

---

## YAML file format

One file per component. Filename is not significant; the `name` field is the
identifier.

```yaml
version: "1"                          # file schema version — currently only "1" is accepted
name: payment-service                 # component identifier (must be unique across all files in dir)
team: payments                        # team identifier (files sharing a team name → same Team entity)
repos:
  - id: PLAT/payment-service          # matches model.Repo.RepoID (project/slug)
    role: cicd                        # one of: ci | cd | cicd  (case-insensitive)
  - id: PLAT/payment-infra
    role: cd
workflows:
  - id: payment-service-build         # matches model.Workflow.ID
    role: ci
  - id: payment-service-deploy
    role: cd
artifacts:                            # artifacts deployed together in the CD step
  - name: payment-service-api         # free-form name
    type: docker-image                # free-form type (docker-image, helm-chart, …)
    repository: registry.example.com/payment-service-api
```

### Fields

| Field | Required | Description |
|---|---|---|
| `version` | yes | File schema version. Only `"1"` accepted. |
| `name` | yes | Component identifier. Must be unique across all files in the directory. |
| `team` | yes | Team name. Multiple files sharing the same team name produce one `model.Team`. |
| `repos` | yes (non-empty) | Repos used by this component. |
| `repos[].id` | yes | `model.Repo.RepoID` — format `PROJECT/repo-slug`. |
| `repos[].role` | yes | `ci`, `cd`, or `cicd` (case-insensitive). |
| `workflows` | yes (non-empty) | Workflows used by this component. |
| `workflows[].id` | yes | `model.Workflow.ID`. |
| `workflows[].role` | yes | `ci`, `cd`, or `cicd` (case-insensitive). |
| `artifacts` | no | CD artifacts deployed together. May be empty. |
| `artifacts[].name` | yes | Human-readable artifact name. |
| `artifacts[].type` | no | Free-form artifact type (e.g. `docker-image`). |
| `artifacts[].repository` | no | Registry or repository path. |

### Validation rules

- Unknown `version` → error.
- Empty or whitespace `name` or `team` → error.
- Empty `repos` or `workflows` list → error.
- Unknown `role` value → error with field path.
- Duplicate `repos[].id` within a file → error.
- Duplicate `workflows[].id` within a file → error.
- Duplicate `name` across files in the same directory → error.

---

## Running

```bash
# standalone
dalikamata ingest config --dir ./components

# integrated (mono mode)
dalikamata mono \
  --bitbucket-url https://bitbucket.example.com \
  --bitbucket-token <token> \
  --components-dir ./components
```

The crawler is one-shot: it reads all files, publishes, then exits. Re-runs
are idempotent (domain repository upserts by name).

---

## Published events

| NATS subject | Payload | Frequency |
|---|---|---|
| `ingest.platform.team` | `model.Team` (JSON) | Once per unique team name |
| `ingest.platform.component` | `model.Component` (JSON) | Once per file |

---

## Querying

After ingestion, use the query DSL to read entities:

```json
{ "entity": "team" }
{ "entity": "component", "filter": { "op": "term", "field": "team_name", "value": "payments" } }
```

Subjects:
- `query.platform.team`
- `query.platform.component`

See `internal/domain/nats/query_client.go` for the typed Go client:
`QueryTeamsAll` / `QueryComponentsAll`.
