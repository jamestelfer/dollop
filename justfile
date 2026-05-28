# Run all checks before committing
verify: fmt build lint test

# Format all Go source files
fmt:
    gofmt -w .

# Run all tests
test *args:
    go test ./... {{args}}

# Build the binary, stamping a dev version derived from the release-please manifest
build *args:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p dist
    base="$(jq -r '.["."]' .release-please-manifest.json)"
    version="${base}-dev.$(git rev-parse --short HEAD)"
    if [ -n "$(git status --porcelain)" ]; then
        version="${version}.dirty"
    fi
    env CGO_ENABLED=0 go build -trimpath \
        -ldflags "-X github.com/jamestelfer/dollop/internal/buildinfo.Version=${version}" \
        -o dist/ ./cmd/dollop/... {{args}}

# Run linter
lint:
    golangci-lint run ./...
