package sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSidecarMissingReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	sc, err := ReadSidecar(filepath.Join(dir, "hello.md"))
	if err != nil {
		t.Fatalf("ReadSidecar missing = %v", err)
	}
	if len(sc) != 0 {
		t.Errorf("want empty sidecar, got %d sections", len(sc))
	}
}

func TestReadSidecarHappy(t *testing.T) {
	dir := t.TempDir()
	yaml := `confluence:
  target_id: "12345"
  last_push:
    doc_hash: "sha256:abc"
    at: "2026-05-26T10:00:00Z"
    remote_revision: "7"
    actor: "claude-code"
  last_check: null
  last_error: null
`
	os.WriteFile(filepath.Join(dir, "hello.integrations.yaml"), []byte(yaml), 0o644)
	sc, err := ReadSidecar(filepath.Join(dir, "hello.md"))
	if err != nil {
		t.Fatalf("ReadSidecar = %v", err)
	}
	s := sc["confluence"]
	if s.TargetID != "12345" {
		t.Errorf("target_id = %q", s.TargetID)
	}
	if s.LastPush == nil || s.LastPush.RemoteRevision != "7" {
		t.Errorf("last_push not parsed: %+v", s.LastPush)
	}
	if s.LastCheck != nil || s.LastError != nil {
		t.Error("nulls should produce nil records")
	}
}

func TestSidecarPathFromMd(t *testing.T) {
	if SidecarPath("docs/notes/hello.md") != "docs/notes/hello.integrations.yaml" {
		t.Fail()
	}
	if SidecarPath("docs/runbooks/db-failover.MD") != "docs/runbooks/db-failover.integrations.yaml" {
		t.Fail()
	}
}
