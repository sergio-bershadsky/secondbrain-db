package sync

import (
	"os"
	"path/filepath"
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
