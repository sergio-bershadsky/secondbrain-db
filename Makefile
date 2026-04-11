.PHONY: build install test test-e2e lint fmt cover clean release-dry-run

BINARY := sbdb
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/sergio-bershadsky/secondbrain-db/internal/version.Version=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BINARY) .

install:
	go install $(LDFLAGS) .

test:
	go test ./... -race -count=1

test-e2e:
	go test -tags=e2e -race -count=1 -timeout=120s ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .
	goimports -w .

cover:
	go test ./internal/... -race -coverprofile=coverage.out -covermode=atomic
	go tool cover -func=coverage.out
	@echo "---"
	@COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | tr -d '%'); \
	echo "Total coverage: $${COVERAGE}%"; \
	if [ $$(echo "$${COVERAGE} < 60" | bc -l) -eq 1 ]; then \
		echo "FAIL: coverage $${COVERAGE}% < 60% threshold"; exit 1; \
	fi

clean:
	rm -f $(BINARY) coverage.out coverage.html
	rm -rf dist/

release-dry-run:
	goreleaser release --snapshot --clean
