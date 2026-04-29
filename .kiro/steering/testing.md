---
inclusion: always
---

# Testing

## Build Tags

- All test files require a build tag. Unit tests use `//go:build unit`. Other tags: `integration`, `sudo`, `sudo_unit`, `e2e`.
- Platform-specific tests combine tags (e.g., `//go:build !windows && unit`, `//go:build linux && sudo_unit`).

## Running Tests

- Run unit tests from the repo root: `make test`. This runs tests in both `agent/` and `ecs-agent/` modules with `-tags unit -mod vendor -count=1`.
- Run a single test or package directly: `go test -tags unit ./path/... -count=1`.
- Some packages may not compile on macOS due to Linux-specific dependencies. Run tests inside a Docker container when needed, for example:
  ```bash
  docker run --rm -v "$(pwd)":/src -w /src/agent golang:$(cat GO_VERSION) \
    go test -tags unit -run TestName ./path/... -count=1
  ```

## Conventions

- Tests must be written in a table-driven format, when there are multiple test cases testing the same function. Aim for 80%+ line coverage on new code.
- Use AWS SDK v2 helpers for type conversions in tests (e.g., `aws.Bool()`, `aws.String()`).

## Mock Generation

- Mocks are generated with `mockgen` via `//go:generate` directives in `generate_mocks.go` files colocated with the source package.
- Mock output goes to a `mocks/` subdirectory within the package.
- All generated mocks include the Apache 2.0 license header via `-copyright_file=../../scripts/copyright_file` (path relative to the generate file).
- Run `make gogenerate` to regenerate all mocks. CI verifies no diff is produced.
