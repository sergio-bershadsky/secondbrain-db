package sync

import (
	"strings"
	"testing"
)

func TestComputeDocHashStable(t *testing.T) {
	a := `---
id: hello
title: Hello
sync:
  confluence:
    pageId: "12345"
---
# Hello

Body.
`
	h1, err := ComputeDocHash([]byte(a))
	if err != nil {
		t.Fatal(err)
	}
	// Add trailing whitespace; hash must not change.
	h2, _ := ComputeDocHash([]byte(a + "   \n\n"))
	if h1 != h2 {
		t.Errorf("trailing-ws change altered hash: %s vs %s", h1, h2)
	}
}

func TestComputeDocHashSensitivity(t *testing.T) {
	a := []byte("---\nid: hello\n---\nbody\n")
	b := []byte("---\nid: hello\n---\nDifferent body\n")
	h1, _ := ComputeDocHash(a)
	h2, _ := ComputeDocHash(b)
	if h1 == h2 {
		t.Error("different bodies produced same hash")
	}
}

func TestComputeDocHashPrefix(t *testing.T) {
	h, err := ComputeDocHash([]byte("---\nid: x\n---\nbody"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "sha256:") {
		t.Errorf("hash %q lacks sha256: prefix", h)
	}
}
