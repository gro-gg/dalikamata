# dalikamata

Collect, generate and evaluate Continuous Delivery Metrics in your Organization.

Dalikamata (or Dalikmata) is a pre-colonial Visayan deity of the Philippines, revered as the goddess of eyes, eyesight, and clairvoyance. Known as a "many-eyed" deity, she is invoked to cure eye ailments, provide prophetic visions, and is believed to shed tears of dew at night over human suffering.

## Docker Compose

Start all services individually (micro):

```bash
docker compose -f deploy/docker/docker-compose-micro.yaml up
```

Or start the domain, ingest and metrics services together as a single process (mono):

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
