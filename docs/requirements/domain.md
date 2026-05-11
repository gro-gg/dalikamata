# Domain Service — Requirements

## Purpose

Provide the event streaming backbone for dalikamata. The domain service runs an embedded NATS JetStream server and manages the ingest stream through which all Git events (repositories, commits, pull requests) flow between ingest services and downstream consumers.

## Architecture

- Embeds a **NATS JetStream server** that persists events to disk.
- Owns the **INGEST stream**, which captures all subjects under `ingest.>`.
- Exposes a **publisher API** (`GITPublisher`) that ingest services use to emit events.
- Runs **durable consumers** to process inbound events.

## Stream

| Attribute | Value |
|-----------|-------|
| Name | `INGEST` |
| Subject filter | `ingest.>` |
| Storage | File (JetStream persistent) |

## Subjects

| Subject | Payload | Description |
|---------|---------|-------------|
| `ingest.git.repo` | `model.Repo` JSON | A repository discovered by an ingestor |
| `ingest.git.commit` | `model.Commit` JSON | A commit discovered by an ingestor |
| `ingest.git.pullrequest` | `model.PullRequest` JSON | A pull request discovered by an ingestor |

## Consumers

### `ingest-git-repo`

| Attribute | Value |
|-----------|-------|
| Subject | `ingest.git.repo` |
| Durable | yes |
| Ack policy | Explicit |
| Max deliver | 5 |

On receipt: deserialises the JSON payload into `model.Repo` and acknowledges. If deserialisation fails, the message is negatively acknowledged and redelivered up to `MaxDeliver` times.

## Publisher API

The `GITPublisher` is the interface ingest services use to emit events into the domain. It connects to the NATS server and provides three methods:

| Method | Subject |
|--------|---------|
| `PublishRepo(ctx, model.Repo)` | `ingest.git.repo` |
| `PublishCommit(ctx, model.Commit)` | `ingest.git.commit` |
| `PublishPullRequest(ctx, model.PullRequest)` | `ingest.git.pullrequest` |

All payloads are serialised to JSON before publishing.

## Data Models

### `model.Repo`
| Field | Type | Description |
|-------|------|-------------|
| `RepoID` | string | Unique repository identifier |
| `Name` | string | Repository name |

### `model.Commit`
| Field | Type | Description |
|-------|------|-------------|
| `SHA` | string | Commit hash |
| `RepoID` | string | Repository the commit belongs to |
| `Author` | string | Commit author |
| `Timestamp` | time.Time | When the commit was made |

### `model.PullRequest`
| Field | Type | Description |
|-------|------|-------------|
| `ID` | string | Pull request identifier |
| `RepoID` | string | Repository the PR belongs to |
| `Name` | string | PR name |
| `Title` | string | PR title |
| `Description` | string | PR description |
| `State` | string | `OPEN`, `MERGED`, or `DECLINED` |
| `Author` | string | PR author |
| `CreatedAt` | time.Time | When the PR was opened |
| `UpdatedAt` | time.Time | When the PR was last updated |

## Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `--nats-host` | `0.0.0.0` | Address the NATS server binds on (persistent root flag) |
| `--nats-port` | `4222` | Port the NATS server listens on (persistent root flag) |
| `--nats-data` | `./data/nats` | Directory for JetStream persistence (persistent root flag) |
| `--nats-server` | `true` | Whether to start the embedded NATS server (persistent root flag) |

## Error Handling

- A NATS server that fails to start within the startup timeout aborts the process.
- A failed NATS client connection aborts startup.
- Messages that fail deserialisation are negatively acknowledged and redelivered; after `MaxDeliver` (5) attempts they are not redelivered further.

## Future Work
