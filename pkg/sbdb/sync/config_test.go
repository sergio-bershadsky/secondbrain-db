package sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigsEmpty(t *testing.T) {
	dir := t.TempDir()
	cfgs, err := LoadConfigs(dir)
	if err != nil {
		t.Fatalf("LoadConfigs empty dir = %v", err)
	}
	if len(cfgs) != 0 {
		t.Errorf("want 0 configs, got %d", len(cfgs))
	}
}

func TestLoadConfigsOne(t *testing.T) {
	dir := t.TempDir()
	yaml := `integration: confluence
applies_to:
  notes:
    target_ref: sync.confluence.pageId
    required: false
    payload:
      title: { from: frontmatter.title }
      body:  { from: rendered_markdown }
`
	if err := os.WriteFile(filepath.Join(dir, "confluence.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgs, err := LoadConfigs(dir)
	if err != nil {
		t.Fatalf("LoadConfigs = %v", err)
	}
	if len(cfgs) != 1 {
		t.Fatalf("want 1 config, got %d", len(cfgs))
	}
	c, ok := cfgs["confluence"]
	if !ok {
		t.Fatal("missing confluence config")
	}
	if c.AppliesTo["notes"].TargetRef != "sync.confluence.pageId" {
		t.Errorf("target_ref = %q", c.AppliesTo["notes"].TargetRef)
	}
	if c.AppliesTo["notes"].Payload["title"].From != "frontmatter.title" {
		t.Errorf("title.from = %q", c.AppliesTo["notes"].Payload["title"].From)
	}
}

func TestLoadConfigsDuplicateIntegrationRejected(t *testing.T) {
	dir := t.TempDir()
	yaml := `integration: confluence
applies_to: { notes: { target_ref: sync.confluence.pageId } }
`
	os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(yaml), 0o644)
	os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(yaml), 0o644)
	if _, err := LoadConfigs(dir); err == nil {
		t.Fatal("want error on duplicate integration name across files")
	}
}

func TestLoadConfigsMissingDir(t *testing.T) {
	cfgs, err := LoadConfigs(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("missing dir should be empty, got err: %v", err)
	}
	if len(cfgs) != 0 {
		t.Errorf("want 0 configs, got %d", len(cfgs))
	}
}
