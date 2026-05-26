package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequiredTargetMissingBlocksCreate(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "schemas"), 0o755)
	os.WriteFile(filepath.Join(root, "schemas", "runbooks.yaml"),
		[]byte("version: 1\nentity: runbooks\ndocs_dir: docs/runbooks\nfilename: \"{id}.md\"\nid_field: id\nintegrity: off\nfields:\n  id: { type: string, required: true }\n"), 0o644)
	os.MkdirAll(filepath.Join(root, ".sbdb", "integrations"), 0o755)
	os.WriteFile(filepath.Join(root, ".sbdb", "integrations", "confluence.yaml"),
		[]byte("integration: confluence\napplies_to:\n  runbooks:\n    target_ref: sync.confluence.pageId\n    required: true\n    payload: {t: {from: frontmatter.title}}\n"), 0o644)

	inputFile := filepath.Join(root, "payload.json")
	os.WriteFile(inputFile, []byte(`{"id": "rb1"}`), 0o644)

	resetFlagsForTest()
	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"create", "--base-path", root, "-s", "runbooks", "--input", inputFile})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("want error: required target_ref missing")
	}
	if !strings.Contains(err.Error(), "sync.confluence.pageId") {
		t.Errorf("error should mention missing path, got: %v", err)
	}
}

func TestDoctorCheckValidatesIntegrationConfigs(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "schemas"), 0o755)
	os.WriteFile(filepath.Join(root, "schemas", "notes.yaml"),
		[]byte("x-entity: notes\nx-storage: {docs_dir: docs/notes}\ntype: object\nproperties: {id: {type: string}}\n"), 0o644)
	os.MkdirAll(filepath.Join(root, ".sbdb", "integrations"), 0o755)
	// Bad config: references unknown entity.
	os.WriteFile(filepath.Join(root, ".sbdb", "integrations", "broken.yaml"),
		[]byte("integration: broken\napplies_to:\n  ghosts:\n    target_ref: x.y\n    payload: {t: {from: frontmatter.title}}\n"), 0o644)

	resetFlagsForTest()
	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"doctor", "check", "--base-path", root})
	if err := cmd.Execute(); err == nil {
		t.Fatal("want error: invalid integration config")
	}
}
