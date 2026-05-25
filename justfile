# Run all checks before committing
verify: fmt build lint test

# Format all Go source files
fmt:
    gofmt -w .

# Run all tests
test *args:
    go test ./... {{args}}

# Build the binary
build *args:
    mkdir -p dist
    env CGO_ENABLED=0 go build -trimpath -o dist/ ./cmd/dollop/... {{args}}

# Run linter
lint:
    golangci-lint run ./...
