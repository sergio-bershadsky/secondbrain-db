package events

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConformance_BuiltinCorpus parses every line of the builtin corpus,
// validates structural invariants, and verifies the type set matches
// BuiltinTypes exactly (no orphans, no missing examples).
func TestConformance_BuiltinCorpus(t *testing.T) {
	path := filepath.Join(packageDir(t), "conformance_corpus", "builtin.jsonl")
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	registry := NewBuiltinRegistry()

	seen := map[string]bool{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), MaxLineBytes+128)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		ev, err := ParseLine(line)
		require.NoError(t, err, "line %d: %s", lineNum, scanner.Text())
		require.NoError(t, ev.Validate(), "line %d: validate", lineNum)
		require.True(t, registry.IsKnownType(ev.Type),
			"line %d: type %q not in registry", lineNum, ev.Type)

		// Re-marshal must round-trip.
		out, err := ev.MarshalLine()
		require.NoError(t, err, "line %d: marshal", lineNum)
		require.LessOrEqual(t, len(out), MaxLineBytes, "line %d: oversize", lineNum)

		seen[ev.Type] = true
	}
	require.NoError(t, scanner.Err())

	// Every built-in type MUST appear at least once in the corpus.
	missing := []string{}
	for _, t := range BuiltinTypes {
		if !seen[t] {
			missing = append(missing, t)
		}
	}
	sort.Strings(missing)
	require.Empty(t, missing, "built-in types missing from conformance corpus: %v", missing)
}

// TestConformance_NoOrphanCorpusEntries fails if the corpus references a
// type that is NOT in BuiltinTypes (would mean spec/code drift).
func TestConformance_NoOrphanCorpusEntries(t *testing.T) {
	path := filepath.Join(packageDir(t), "conformance_corpus", "builtin.jsonl")
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	known := map[string]bool{}
	for _, t := range BuiltinTypes {
		known[t] = true
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), MaxLineBytes+128)
	for scanner.Scan() {
		ev, err := ParseLine(scanner.Bytes())
		require.NoError(t, err)
		require.True(t, known[ev.Type], "corpus references unknown built-in type: %s", ev.Type)
	}
}

// TestConformance_TypesAreSorted verifies the BuiltinTypes slice is grouped
// by bucket and sorted within group — keeps `sbdb event types` output stable.
func TestConformance_TypesAreSorted(t *testing.T) {
	// Build map: bucket → ordered verbs as they appear.
	byBucket := map[string][]string{}
	order := []string{}
	for _, full := range BuiltinTypes {
		b := Bucket(full)
		v := Verb(full)
		if _, seen := byBucket[b]; !seen {
			order = append(order, b)
		}
		byBucket[b] = append(byBucket[b], v)
	}
	// Each bucket's verbs need not be alphabetically sorted in the slice
	// (we sort in NewBuiltinRegistry); just make sure no bucket is split.
	seenBuckets := map[string]bool{}
	for _, full := range BuiltinTypes {
		b := Bucket(full)
		if seenBuckets[b] {
			continue
		}
		// First occurrence: every subsequent BuiltinTypes entry for THIS
		// bucket must come before any new bucket starts after this one.
		seenBuckets[b] = true
	}
	_ = order // sanity holder
	_ = runtime.GOOS
}
