# dalikamata

Collect, generate and evaluate Continuous Delivery Metrics in your Organization.

Dalikamata (or Dalikmata) is a pre-colonial Visayan deity of the Philippines, revered as the goddess of eyes, eyesight, and clairvoyance. Known as a "many-eyed" deity, she is invoked to cure eye ailments, provide prophetic visions, and is believed to shed tears of dew at night over human suffering.

## Commands

| Command | Description |
|---|---|
| `dalikamata nats` | Start the embedded NATS JetStream server |
| `dalikamata domain` | Start the domain service (requires a running NATS server) |
| `dalikamata metrics` | Start the metrics service (requires a running NATS server) |
| `dalikamata ingest bitbucket` | Crawl Bitbucket and publish events to NATS |
| `dalikamata ingest jenkins` | Crawl Jenkins and publish pipeline events to NATS |
| `dalikamata mono` | Start NATS, domain, metrics, and ingest together in one process |

Key flags (available on all commands via root):

| Flag | Default | Description |
|---|---|---|
| `--nats-host` | `0.0.0.0` | NATS server hostname to connect to |
| `--nats-port` | `4222` | NATS server port |
| `--debug` | `false` | Enable debug logging |
| `--grace-period` | `10s` | Shutdown grace period |
| `--metrics-addr` | `0.0.0.0:2112` | Prometheus metrics listen address |

The `nats` and `mono` commands also accept `--nats-data` (default `./data/nats`) to set the JetStream persistence directory.

## Docker Compose

Start all services individually (micro) — each service runs in its own container, with NATS in a dedicated container:

```bash
docker compose -f deploy/docker/docker-compose-micro.yaml up
```

Or start NATS, domain, ingest, and metrics together as a single process (mono):

```bash
docker compose -f deploy/docker/docker-compose-mono.yaml up
```

# Testing

### Unit tests

```bash
go test ./... -race
```

### Integration tests

Integration tests start real service subprocesses (`go run .`) and verify
end-to-end behaviour. They are gated behind the `integration` build tag so they
do not run during normal `go test ./...` invocations.

```bash
go test -tags=integration ./internal/ingest/bitbucket/... -v -timeout 20s
```
