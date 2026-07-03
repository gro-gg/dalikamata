# AGENTS.md

This file provides guidance to coding agents and human contributors when working in this repository.

## Verification pipeline (Definition of Done)

After every code change, run these steps **in order**. A change is not done until they all pass:

```bash
# 0. build
go build ./...

# 1. test
go test ./... -race

# 2. lint
golangci-lint run

# 3. fmt
go fmt ./...

# 4. e2e-test
go test -tags=e2e ./internal/e2e/... -v -timeout 2m -skip-docker-build

```

If any step fails, fix it (or report the failure with its output) before considering the change complete.
