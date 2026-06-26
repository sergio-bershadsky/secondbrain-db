package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDocIntegration(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "hello.md")
	doc := []byte(`---
id: hello
title: Hello
tags: [intro]
sync:
  confluence:
    pageId: "12345"
---
# Hello

Body text.
`)
	os.WriteFile(docPath, doc, 0o644)

	cfg := IntegrationConfig{
		Integration: "confluence",
		AppliesTo: map[string]EntityBinding{
			"notes": {
				TargetRef: "sync.confluence.pageId",
				Payload: map[string]PayloadField{
					"title":  {From: "frontmatter.title"},
					"labels": {From: "frontmatter.tags"},
					"body":   {From: "rendered_markdown"},
				},
			},
		},
	}
	r, err := ResolveDocIntegration(docPath, "notes", cfg)
	if err != nil {
		t.Fatalf("ResolveDocIntegration = %v", err)
	}
	if r.TargetID != "12345" {
		t.Errorf("target_id = %q", r.TargetID)
	}
	if r.Payload["title"] != "Hello" {
		t.Errorf("payload.title = %v", r.Payload["title"])
	}
	if r.Payload["body"] == "" {
		t.Errorf("payload.body empty")
	}
	if r.DriftResult != ResultNeverPublished {
		t.Errorf("drift = %v, want never_published", r.DriftResult)
	}
}

func TestResolveConstPayload(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "rb.md")
	os.WriteFile(docPath, []byte("---\nid: rb\nsync: {confluence: {pageId: \"1\"}}\n---\nbody\n"), 0o644)
	cfg := IntegrationConfig{
		Integration: "confluence",
		AppliesTo: map[string]EntityBinding{
			"runbooks": {
				TargetRef: "sync.confluence.pageId",
				Payload: map[string]PayloadField{
					"labels": {Const: []interface{}{"runbook"}},
				},
			},
		},
	}
	r, err := ResolveDocIntegration(docPath, "runbooks", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if labels, ok := r.Payload["labels"].([]interface{}); !ok || len(labels) != 1 || labels[0] != "runbook" {
		t.Errorf("labels const = %v", r.Payload["labels"])
	}
}

func TestResolveTargetMissingButOptional(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "hello.md")
	os.WriteFile(docPath, []byte("---\nid: hello\n---\nbody\n"), 0o644)
	cfg := IntegrationConfig{
		Integration: "confluence",
		AppliesTo:   map[string]EntityBinding{"notes": {TargetRef: "sync.confluence.pageId", Required: false, Payload: map[string]PayloadField{"t": {From: "frontmatter.title"}}}},
	}
	r, err := ResolveDocIntegration(docPath, "notes", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r.TargetID != "" {
		t.Errorf("target_id should be empty, got %q", r.TargetID)
	}
	if r.Status != StatusUnlinked {
		t.Errorf("status = %v, want unlinked", r.Status)
	}
}

func TestResolvePayloadFrontmatterMissingIntermediate(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "hello.md")
	// Doc has no `meta` key in frontmatter at all.
	if err := os.WriteFile(docPath, []byte("---\nid: hello\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := IntegrationConfig{
		Integration: "confluence",
		AppliesTo: map[string]EntityBinding{
			"notes": {TargetRef: "sync.confluence.pageId", Payload: map[string]PayloadField{
				"deep": {From: "frontmatter.meta.nested.value"},
			}},
		},
	}
	r, err := ResolveDocIntegration(docPath, "notes", cfg)
	if err != nil {
		t.Fatalf("ResolveDocIntegration = %v", err)
	}
	if r.Payload["deep"] != nil {
		t.Errorf("nested-missing path should resolve to nil, got %v", r.Payload["deep"])
	}
}

func TestResolvedDocIntegrationJSONStateTags(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "hello.md")
	if err := os.WriteFile(docPath, []byte("---\nid: hello\ntitle: Hello\nsync: {confluence: {pageId: \"123\"}}\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Record a push so the state section is populated.
	if err := RecordPush(docPath, "confluence", "123", PushRecord{
		DocHash: "sha256:x", At: "2026-05-29T10:00:00Z", RemoteRevision: "1", Actor: "claude-code",
	}); err != nil {
		t.Fatal(err)
	}
	cfg := IntegrationConfig{
		Integration: "confluence",
		AppliesTo:   map[string]EntityBinding{"notes": {TargetRef: "sync.confluence.pageId", Payload: map[string]PayloadField{"title": {From: "frontmatter.title"}}}},
	}
	r, err := ResolveDocIntegration(docPath, "notes", cfg)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	s := string(blob)
	// State sub-object must use snake_case JSON tags, not Go field names.
	for _, want := range []string{`"last_push"`, `"doc_hash"`, `"remote_revision"`} {
		if !strings.Contains(s, want) {
			t.Errorf("JSON missing %s; got: %s", want, s)
		}
	}
	for _, bad := range []string{`"LastPush"`, `"DocHash"`, `"RemoteRevision"`, `"TargetID"`} {
		if strings.Contains(s, bad) {
			t.Errorf("JSON leaked Go field name %s; got: %s", bad, s)
		}
	}
}
