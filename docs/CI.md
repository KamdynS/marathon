## GitHub Actions: CI for Marathon (Testing + Coverage)

This doc provides a ready-to-use CI workflow for the `marathon/` module that runs tests (with race detector), collects coverage, and caches dependencies. Keep this as a reference until CI is enabled publicly.

### Goals
- Fast feedback on pushes and PRs
- Table-driven tests, race-safety (`-race`), and coverage reporting
- Dependency caching for `go` and module downloads
- Monorepo-safe: runs only for changes under `marathon/`

### Recommended Workflow (copy to `.github/workflows/ci.yml`)

```yaml
name: ci

on:
  pull_request:
    paths:
      - 'marathon/**'
      - '.github/workflows/ci.yml'
  push:
    branches: [ main ]
    paths:
      - 'marathon/**'
      - '.github/workflows/ci.yml'

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  test:
    name: Go Tests (Race + Coverage)
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: marathon
    strategy:
      fail-fast: false
      matrix:
        go: ['1.22.x', '1.23.x']
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}

      - name: Cache Go build
        uses: actions/cache@v4
        with:
          path: |
            ~/.cache/go-build
            ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ matrix.go }}-${{ hashFiles('**/go.sum') }}
          restore-keys: |
            ${{ runner.os }}-go-${{ matrix.go }}-

      - name: Download deps
        run: go mod download

      - name: Vet
        run: go vet ./...

      - name: Test (race + coverage)
        run: go test ./... -race -cover -coverprofile=coverage.out

      - name: Coverage summary
        run: |
          go tool cover -func=coverage.out | tail -n 1

      - name: Upload coverage artifact
        uses: actions/upload-artifact@v4
        with:
          name: coverage-${{ matrix.go }}
          path: marathon/coverage.out
          if-no-files-found: warn
```

### Notes
- Working directory is set to `marathon/` to avoid running other modules in the repo.
- The matrix runs on Go 1.22 and 1.23; adjust as needed.
- Add per-package coverage gates later if desired (e.g., `go tool cover -func=coverage.out | awk ...`).
- Keep tests deterministic and table-driven (see `CONTRIBUTING.md`).

### Local parity

Run locally what CI runs:

```bash
cd marathon
go vet ./...
go test ./... -race -cover -coverprofile=coverage.out
go tool cover -func=coverage.out | tail -n 1
```


