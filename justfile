# Run all checks before committing
verify: fmt build test

# Format all Go source files
fmt:
    mise exec -- gofmt -w .

# Run all tests
test *args:
    mise exec -- go test ./... {{args}}

# Build the binary
build *args:
    mkdir -p dist
    mise exec -- env CGO_ENABLED=0 go build -trimpath -o dist/ ./cmd/imds-broker/... {{args}}

# Run linter
#lint:
#    mise exec -- golangci-lint run ./...
