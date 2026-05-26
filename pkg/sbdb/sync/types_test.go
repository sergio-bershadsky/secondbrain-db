package sync

import "testing"

func TestDriftResultStringer(t *testing.T) {
	cases := map[DriftResult]string{
		ResultInSync:         "in_sync",
		ResultLocalDrift:     "local_drift",
		ResultRemoteDrift:    "remote_drift",
		ResultBothDrift:      "both_drift",
		ResultNeverPublished: "never_published",
	}
	for r, want := range cases {
		if got := r.String(); got != want {
			t.Errorf("%v.String() = %q, want %q", r, got, want)
		}
	}
}

func TestDriftResultParse(t *testing.T) {
	if r, err := ParseDriftResult("local_drift"); err != nil || r != ResultLocalDrift {
		t.Fatalf("ParseDriftResult(local_drift) = %v, %v", r, err)
	}
	if _, err := ParseDriftResult("nonsense"); err == nil {
		t.Fatal("ParseDriftResult(nonsense) should error")
	}
}
