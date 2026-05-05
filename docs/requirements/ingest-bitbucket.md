# Ingest Bitbucket Service — Requirements

## Purpose

Crawl a self-hosted Bitbucket Server instance and publish information about commits and pull requests to the domain serivce via NATS.

## Architecture

- Ingest services consume data from external sources and forward it to the domain serivce.
- Communication between ingest and domain serivces happens via **NATS**.
- Deduplication is handled by the domain serivce; the ingest service publishes all data without deduplication.
- Domain model types are defined in `pkg/model` (onion architecture); ingest packages map from source-specific API types to these domain types.

## Bitbucket Target

- **Bitbucket Server** (self-hosted), not Bitbucket Cloud.
- Authentication via **Personal Access Token (PAT)**.

## Crawling Strategy

- Perform a **full crawl at startup** over a configured set of projects.
- Projects are currently **hardcoded** (configurable via CLI flags).
- Webhook-based incremental updates are planned for a future iteration.

## Scope

For each configured project:
1. Fetch all repositories.
2. For each repository, fetch all **commits** and all **pull requests** (all states: OPEN, MERGED, DECLINED).

## Data Model

Domain types are defined in `pkg/model/git.go`. The ingest service maps Bitbucket API responses to these types before publishing. |

## NATS Subjects

| Subject                   | Payload                  |
|---------------------------|--------------------------|
| `ingest.git.repo`         | `model.Repo` JSON        |
| `ingest.git.commit`       | `model.Commit` JSON      |
| `ingest.git.pullrequest`  | `model.PullRequest` JSON |

## CLI Flags (`dalikamata ingest bitbucket`)

| Flag                 | Required | Description                                        |
|----------------------|----------|----------------------------------------------------|
| `--bitbucket-url`    | yes      | Bitbucket Server base URL                          |
| `--bitbucket-token`  | yes      | Personal access token                              |
| `--nats-url`         | no       | NATS server URL (default: `nats://127.0.0.1:4222`) |
| `--projects`         | yes      | Comma-separated list of project keys               |

## Error Handling

- Errors on a single project or repository are logged and skipped; the crawl continues.
- Errors publishing individual events are logged and skipped.
- A failed NATS connection aborts startup.

## Future Work

- Webhook listener for incremental updates (replaces/supplements full crawl).
- Configurable project list (e.g. from config file).