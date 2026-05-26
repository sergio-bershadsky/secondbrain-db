package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncTargets(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "schemas"), 0o755)
	os.WriteFile(filepath.Join(root, "schemas", "notes.yaml"),
		[]byte("x-entity: notes\nx-storage: {docs_dir: docs/notes}\ntype: object\nproperties: {id: {type: string}}\n"), 0o644)
	os.MkdirAll(filepath.Join(root, ".sbdb", "integrations"), 0o755)
	os.WriteFile(filepath.Join(root, ".sbdb", "integrations", "confluence.yaml"),
		[]byte("integration: confluence\napplies_to:\n  notes:\n    target_ref: sync.confluence.pageId\n    required: true\n    payload: {t: {from: frontmatter.title}}\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "docs", "notes"), 0o755)
	os.WriteFile(filepath.Join(root, "docs", "notes", "hello.md"),
		[]byte("---\nid: hello\nsync: {confluence: {pageId: \"12345\"}}\n---\nbody\n"), 0o644)

	resetFlagsForTest()
	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"sync", "targets", "--base-path", root, "-s", "notes", "--id", "hello"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\noutput: %s", err, buf.String())
	}
	var got map[string]map[string]string
	json.Unmarshal(buf.Bytes(), &got)
	if got["confluence"]["target_id"] != "12345" {
		t.Errorf("target_id = %q", got["confluence"]["target_id"])
	}
	if got["confluence"]["status"] != "linked" {
		t.Errorf("status = %q", got["confluence"]["status"])
	}
}
