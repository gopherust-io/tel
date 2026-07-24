# Contributing

## Prerequisites

- Go version from [`go.mod`](go.mod)
- `make` helpers (optional)

## Development

```bash
make test             # unit tests
make test-race        # race detector
make lint             # govulncheck + golangci-lint
make examples         # build examples
```

## Pull requests

1. Keep changes focused; match existing package style (small interfaces, table-driven tests).
2. Run `make fmt-check`, `make test`, and `make lint` before opening a PR.
3. Update `README.md` when changing public APIs.
4. Do not commit secrets.

CI on PRs runs format, vet, unit tests, examples build, and golangci-lint. Race, benchmarks, and govulncheck run on pushes to `main`.
