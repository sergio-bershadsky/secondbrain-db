package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupBasicProject(t *testing.T) string {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "schemas"), 0o755)
	os.WriteFile(filepath.Join(root, "schemas", "notes.yaml"),
		[]byte("x-entity: notes\nx-storage: {docs_dir: docs/notes}\ntype: object\nproperties: {id: {type: string}}\n"), 0o644)
	os.MkdirAll(filepath.Join(root, ".sbdb", "integrations"), 0o755)
	os.WriteFile(filepath.Join(root, ".sbdb", "integrations", "confluence.yaml"),
		[]byte("integration: confluence\napplies_to:\n  notes:\n    target_ref: sync.confluence.pageId\n    payload: {t: {from: frontmatter.title}}\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "docs", "notes"), 0o755)
	os.WriteFile(filepath.Join(root, "docs", "notes", "hello.md"),
		[]byte("---\nid: hello\ntitle: Hello\nsync: {confluence: {pageId: \"12345\"}}\n---\nbody\n"), 0o644)
	return root
}

func TestSyncStateGetEmpty(t *testing.T) {
	root := setupBasicProject(t)
	resetFlagsForTest()
	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"sync", "state", "get", "--base-path", root, "-s", "notes", "--id", "hello", "--integration", "confluence"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\n%s", err, buf.String())
	}
	var got map[string]interface{}
	json.Unmarshal(buf.Bytes(), &got)
	if got["target_id"] != "12345" {
		t.Errorf("target_id = %v", got["target_id"])
	}
}

func TestSyncStateSetPush(t *testing.T) {
	root := setupBasicProject(t)
	resetFlagsForTest()
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"sync", "state", "set", "--base-path", root, "-s", "notes", "--id", "hello", "--integration", "confluence",
		"--published-hash", "sha256:newhash", "--remote-revision", "7", "--at", "2026-05-26T10:00:00Z"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(root, "docs", "notes", "hello.integrations.yaml"))
	if !bytes.Contains(data, []byte("sha256:newhash")) {
		t.Errorf("sidecar lacks new hash:\n%s", data)
	}
	if !bytes.Contains(data, []byte("remote_revision: \"7\"")) {
		t.Errorf("sidecar lacks remote_revision:\n%s", data)
	}
}

func TestSyncStateSetErrorPreservesPush(t *testing.T) {
	root := setupBasicProject(t)
	// First, record a successful push.
	resetFlagsForTest()
	cmd := newRootCmd()
	cmd.SetArgs([]string{"sync", "state", "set", "--base-path", root, "-s", "notes", "--id", "hello", "--integration", "confluence",
		"--published-hash", "sha256:original", "--remote-revision", "1", "--at", "2026-05-26T09:00:00Z"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.Execute()

	// Then, record an error.
	resetFlagsForTest()
	cmd = newRootCmd()
	cmd.SetArgs([]string{"sync", "state", "set", "--base-path", root, "-s", "notes", "--id", "hello", "--integration", "confluence",
		"--error", "401 expired", "--stage", "push", "--at", "2026-05-26T10:00:00Z"})
	cmd.SetOut(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(root, "docs", "notes", "hello.integrations.yaml"))
	if !bytes.Contains(data, []byte("sha256:original")) {
		t.Errorf("error overwrote successful last_push:\n%s", data)
	}
	if !bytes.Contains(data, []byte("401 expired")) {
		t.Errorf("last_error missing:\n%s", data)
	}
}
