# Ingest Jenkins Service — Requirements

## Purpose

Crawl a self-hosted Jenkins server and publish information about pipeline builds and stages to the domain service via NATS.

## Architecture

- Ingest services consume data from external sources and forward it to the domain service.
- Communication between ingest and domain services happens via **NATS**.
- Deduplication is handled by the domain service; the ingest service publishes all data without deduplication.
- Domain model types are defined in `pkg/model` (onion architecture); ingest packages map from source-specific API types to these domain types.

## Jenkins Target

- **Jenkins** (self-hosted), not CloudBees SaaS.
- Authentication via **username + API token**.
- TLS for instances behind an internal CA is configured via the global `--ca-certs-dir` flag.

## Crawling Strategy

- Perform a **full crawl at startup**.
- If `--jenkins-jobs` is provided, crawl only those jobs (values are matched against the **full Jenkins job path**).
- If `--jenkins-jobs` is omitted, **discover all jobs** on the server first, then crawl each.
- Job discovery recurses into **folders** and expands **multibranch pipelines** into their per-branch sub-jobs.
- **Only pipeline jobs** (declarative or scripted) are crawled. Freestyle, matrix, and multi-config jobs are skipped.
- For each job: fetch all completed builds. For each build: fetch its pipeline stages.
- **In-progress builds are skipped**; they will be picked up once they complete.

## Scope

For each job in scope:
1. Publish a `Job` event describing the job (full path + leaf name).
2. Fetch all builds (number, status, timestamps, SCM info).
3. For each build, fetch all **pipeline stages** (name, status, duration).

**Multibranch pipelines** are expanded into their per-branch sub-jobs; each branch is crawled as an independent job. **Folders** are traversed recursively to discover jobs at any depth. A job's identity is its **full Jenkins path** (e.g. `team/payments/main`), not its leaf name — leaf names collide across folders and branches.

## Data Model

Pipeline domain types are defined in `pkg/model/pipeline.go`. The ingest service maps Jenkins API responses to these types before publishing.

### New types

```go
// Build status constants
const (
    BuildStatusSuccess  = "SUCCESS"
    BuildStatusFailure  = "FAILURE"
    BuildStatusAborted  = "ABORTED"
    BuildStatusUnstable = "UNSTABLE"
)

type Job struct {
    JobID string `json:"job_id"` // full Jenkins path, e.g. "team/payments/main"
    Name  string `json:"name"`   // leaf name, e.g. "main"
}

type Build struct {
    ID        string    `json:"id"`         // "<job-id>/<build-number>"
    JobID     string    `json:"job_id"`     // full Jenkins path of the owning job
    Number    int       `json:"number"`
    Status    string    `json:"status"`
    Branch    string    `json:"branch"`     // from SCM info; empty if unavailable
    CommitSHA string    `json:"commit_sha"` // from SCM info; empty if unavailable
    StartedAt time.Time `json:"started_at"`
    Duration  float64   `json:"duration"`   // seconds
}

type PipelineStage struct {
    BuildID   string    `json:"build_id"`
    Order     int       `json:"order"` // 0-based execution order within the build
    Name      string    `json:"name"`
    Status    string    `json:"status"`
    StartedAt time.Time `json:"started_at"`
    Duration  float64   `json:"duration"` // seconds
}
```

## NATS Subjects

Subjects publish to the existing `INGEST` JetStream stream (filter `ingest.>`). They are intentionally generic so that future ingestors for other CI/CD tools (GitHub Actions, GitLab CI, etc.) can publish to the same subjects.

| Subject                   | Payload                       |
|---------------------------|-------------------------------|
| `ingest.pipeline.job`     | `model.Job` JSON              |
| `ingest.pipeline.build`   | `model.Build` JSON            |
| `ingest.pipeline.stage`   | `model.PipelineStage` JSON    |

## CLI Flags (`dalikamata ingest jenkins`)

| Flag              | Required | Description                                                                                          |
|-------------------|----------|------------------------------------------------------------------------------------------------------|
| `--jenkins-url`   | yes      | Jenkins base URL                                                                                     |
| `--jenkins-user`  | yes      | Jenkins username                                                                                     |
| `--jenkins-token` | yes      | Jenkins API token                                                                                    |
| `--nats-url`      | no       | NATS server URL (default: `nats://127.0.0.1:4222`)                                                   |
| `--jenkins-jobs`  | no       | Comma-separated list of **full Jenkins job paths** (e.g. `team/payments/main`); crawl all if omitted |

## Error Handling

- Errors on a single job or build are logged and skipped; the crawl continues.
- Errors publishing individual events are logged and skipped.
- A failed NATS connection aborts startup.

## Future Work

- Scheduled polling for incremental updates (replaces/supplements full crawl with periodic delta fetches).
- Configurable depth limit for folder/multibranch traversal.
- Configurable job list from a config file.
