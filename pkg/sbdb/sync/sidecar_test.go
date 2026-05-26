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

func TestWriteSidecarRoundTrip(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "hello.md")
	sc := Sidecar{
		"confluence": SidecarSection{
			TargetID: "12345",
			LastPush: &PushRecord{DocHash: "sha256:abc", At: "2026-05-26T10:00:00Z", RemoteRevision: "7", Actor: "claude-code"},
		},
	}
	if err := WriteSidecar(md, sc); err != nil {
		t.Fatalf("WriteSidecar = %v", err)
	}
	got, err := ReadSidecar(md)
	if err != nil {
		t.Fatal(err)
	}
	if got["confluence"].LastPush.RemoteRevision != "7" {
		t.Errorf("round-trip lost remote_revision")
	}
}

func TestRecordPushPreservesPriorOnFailure(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "hello.md")
	prior := Sidecar{
		"confluence": SidecarSection{
			TargetID: "12345",
			LastPush: &PushRecord{DocHash: "sha256:old", At: "2026-05-20T10:00:00Z", RemoteRevision: "1", Actor: "claude"},
		},
	}
	WriteSidecar(md, prior)

	// Failed push: must NOT overwrite LastPush.
	err := RecordError(md, "confluence", ErrorRecord{
		At: "2026-05-26T11:00:00Z", Stage: "push", Message: "401 expired", AttemptedDocHash: "sha256:new",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := ReadSidecar(md)
	s := got["confluence"]
	if s.LastPush == nil || s.LastPush.RemoteRevision != "1" {
		t.Errorf("RecordError clobbered LastPush: %+v", s.LastPush)
	}
	if s.LastError == nil || s.LastError.Message != "401 expired" {
		t.Errorf("LastError not written: %+v", s.LastError)
	}
}

func TestRecordPushClearsError(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "hello.md")
	WriteSidecar(md, Sidecar{
		"confluence": SidecarSection{TargetID: "12345", LastError: &ErrorRecord{Message: "old"}},
	})
	err := RecordPush(md, "confluence", "12345", PushRecord{
		DocHash: "sha256:new", At: "2026-05-26T12:00:00Z", RemoteRevision: "2", Actor: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := ReadSidecar(md)
	if got["confluence"].LastError != nil {
		t.Errorf("RecordPush did not clear LastError: %+v", got["confluence"].LastError)
	}
}

func TestRecordCheckPreservesPushAndError(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "hello.md")
	prior := Sidecar{
		"confluence": SidecarSection{
			TargetID:  "12345",
			LastPush:  &PushRecord{DocHash: "sha256:abc", At: "2026-05-20T10:00:00Z", RemoteRevision: "1", Actor: "claude"},
			LastError: &ErrorRecord{At: "2026-05-21T10:00:00Z", Stage: "push", Message: "old failure"},
		},
	}
	if err := WriteSidecar(md, prior); err != nil {
		t.Fatal(err)
	}
	if err := RecordCheck(md, "confluence", CheckRecord{
		At: "2026-05-26T10:00:00Z", Result: "local_drift", CurrentDocHash: "sha256:def",
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := ReadSidecar(md)
	s := got["confluence"]
	if s.LastPush == nil || s.LastPush.RemoteRevision != "1" {
		t.Errorf("RecordCheck clobbered LastPush: %+v", s.LastPush)
	}
	if s.LastError == nil || s.LastError.Message != "old failure" {
		t.Errorf("RecordCheck clobbered LastError: %+v", s.LastError)
	}
	if s.LastCheck == nil || s.LastCheck.Result != "local_drift" {
		t.Errorf("RecordCheck did not write LastCheck: %+v", s.LastCheck)
	}
}
