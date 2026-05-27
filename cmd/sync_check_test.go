package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncCheckNeverPublished(t *testing.T) {
	root := t.TempDir()
	// schemas/
	os.MkdirAll(filepath.Join(root, "schemas"), 0o755)
	os.WriteFile(filepath.Join(root, "schemas", "notes.yaml"),
		[]byte("$schema: https://json-schema.org/draft/2020-12/schema\nx-entity: notes\nx-storage: {docs_dir: docs/notes, filename: \"{id}.md\"}\nx-id: id\ntype: object\nproperties: {id: {type: string}, sync: {type: object}}\nrequired: [id]\n"), 0o644)
	// .sbdb/integrations/confluence.yaml
	os.MkdirAll(filepath.Join(root, ".sbdb", "integrations"), 0o755)
	os.WriteFile(filepath.Join(root, ".sbdb", "integrations", "confluence.yaml"),
		[]byte("integration: confluence\napplies_to:\n  notes:\n    target_ref: sync.confluence.pageId\n    payload: {title: {from: frontmatter.title}}\n"), 0o644)
	// docs/notes/hello.md
	os.MkdirAll(filepath.Join(root, "docs", "notes"), 0o755)
	os.WriteFile(filepath.Join(root, "docs", "notes", "hello.md"),
		[]byte("---\nid: hello\ntitle: Hello\nsync:\n  confluence:\n    pageId: \"12345\"\n---\nbody\n"), 0o644)

	resetFlagsForTest()
	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"sync", "check", "--base-path", root, "--format", "json"})
	err := cmd.Execute()
	// Drift is present (never_published) → non-zero exit. cobra returns the error.
	if err == nil {
		t.Fatal("want non-zero exit (drift present)")
	}
	var rep struct {
		Docs []struct {
			Schema       string                       `json:"schema"`
			ID           string                       `json:"id"`
			Integrations map[string]map[string]string `json:"integrations"`
		} `json:"docs"`
	}
	if jerr := json.Unmarshal(buf.Bytes(), &rep); jerr != nil {
		t.Fatalf("parse output: %v\noutput:\n%s", jerr, buf.String())
	}
	if len(rep.Docs) != 1 || rep.Docs[0].ID != "hello" {
		t.Fatalf("unexpected docs: %+v", rep.Docs)
	}
	if rep.Docs[0].Integrations["confluence"]["result"] != "never_published" {
		t.Errorf("result = %q", rep.Docs[0].Integrations["confluence"]["result"])
	}
}
