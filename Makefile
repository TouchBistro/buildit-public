.DEFAULT_GOAL = build

# Get all dependencies
setup:
# Only install if missing
ifeq (,$(wildcard bin/golangci-lint))
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v2.5.0
endif

	go mod tidy
.PHONY: setup

# Build buildit
build:
	go build
.PHONY: build

# Clean all build artifacts
clean:
	rm -rf dist
	rm -rf coverage
	rm buildit
.PHONY: clean

# Run the linter
lint:
	./bin/golangci-lint --concurrency 4 --timeout 10m run ./...
.PHONY: lint

# Remove version of buildit installed with go install
go-uninstall:
	rm $(shell go env GOPATH)/bin/buildit
.PHONY: go-uninstall

# Run tests and collect coverage data
test:
	mkdir -p coverage
	go test -coverpkg=./... -coverprofile=coverage/coverage.txt ./... 
	go tool cover -html=coverage/coverage.txt -o coverage/coverage.html
.PHONY: test

# Run tests and print coverage data to stdout
test-ci:
	mkdir -p coverage
	go test -coverpkg=./... -coverprofile=coverage/coverage.txt ./...
	go tool cover -func=coverage/coverage.txt
.PHONY: test-ci
