// Package sync implements bookkeeping for external-service mirrors of
// sbdb documents. It owns integration configs, computes local drift,
// and reads/writes per-doc state sidecars.
//
// It does NOT make network calls. Pushing to Confluence/Jira/Slack is
// the job of an integration runtime (Claude + MCP) that consumes this
// package's outputs and reports back via SidecarStore.
package sync
