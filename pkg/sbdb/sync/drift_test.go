package sync

import "testing"

func TestDriftNeverPublished(t *testing.T) {
	r := ComputeDrift(SidecarSection{}, "sha256:current", "")
	if r != ResultNeverPublished {
		t.Errorf("empty section = %v, want never_published", r)
	}
}

func TestDriftInSync(t *testing.T) {
	s := SidecarSection{LastPush: &PushRecord{DocHash: "sha256:same", RemoteRevision: "7"}}
	r := ComputeDrift(s, "sha256:same", "7")
	if r != ResultInSync {
		t.Errorf("matched hash+revision = %v, want in_sync", r)
	}
}

func TestDriftLocalOnly(t *testing.T) {
	s := SidecarSection{LastPush: &PushRecord{DocHash: "sha256:old", RemoteRevision: "7"}}
	r := ComputeDrift(s, "sha256:new", "7")
	if r != ResultLocalDrift {
		t.Errorf("local-only drift = %v", r)
	}
}

func TestDriftRemoteOnly(t *testing.T) {
	s := SidecarSection{LastPush: &PushRecord{DocHash: "sha256:same", RemoteRevision: "7"}}
	r := ComputeDrift(s, "sha256:same", "8")
	if r != ResultRemoteDrift {
		t.Errorf("remote-only drift = %v", r)
	}
}

func TestDriftBoth(t *testing.T) {
	s := SidecarSection{LastPush: &PushRecord{DocHash: "sha256:old", RemoteRevision: "7"}}
	r := ComputeDrift(s, "sha256:new", "8")
	if r != ResultBothDrift {
		t.Errorf("both drift = %v", r)
	}
}

func TestDriftRemoteRevisionEmptySkipsRemoteCheck(t *testing.T) {
	s := SidecarSection{LastPush: &PushRecord{DocHash: "sha256:same", RemoteRevision: "7"}}
	if r := ComputeDrift(s, "sha256:same", ""); r != ResultInSync {
		t.Errorf("local-only check, matching hash = %v, want in_sync", r)
	}
	if r := ComputeDrift(s, "sha256:new", ""); r != ResultLocalDrift {
		t.Errorf("local-only check, diff hash = %v, want local_drift", r)
	}
}
