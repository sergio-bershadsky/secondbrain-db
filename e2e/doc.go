// Package e2e contains end-to-end integration tests that exercise the
// built sbdb binary. All test files are guarded by `//go:build e2e` and
// only run via `go test -tags=e2e ./e2e/...` (or `make test-e2e`).
//
// This file exists without a build tag so `go vet ./...` and `go build
// ./...` see a non-empty package on every run; otherwise tooling reports
// "build constraints exclude all Go files" against the e2e directory.
package e2e
