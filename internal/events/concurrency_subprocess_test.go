package events

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// SBDB_TEST_APPEND_HELPER drives the subprocess writer entry-point. When set,
// the test binary runs as a writer that appends N events to <root> with
// writer-id derived from SBDB_TEST_WRITER_ID, then exits.
const helperEnv = "SBDB_TEST_APPEND_HELPER"

// TestMain dispatches into the subprocess writer when invoked with the env var.
func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) == "1" {
		runHelper()
		return
	}
	os.Exit(m.Run())
}

func runHelper() {
	root := os.Getenv("SBDB_TEST_APPEND_ROOT")
	count, _ := strconv.Atoi(os.Getenv("SBDB_TEST_APPEND_COUNT"))
	writerID, _ := strconv.Atoi(os.Getenv("SBDB_TEST_WRITER_ID"))

	app := NewAppender(root, 1_000_000)
	defer app.Close()
	em := NewEmitter(app, NewBuiltinRegistry())

	for i := 1; i <= count; i++ {
		ev := &Event{
			TS:   time.Now().UTC(),
			Type: "note.created",
			ID:   fmt.Sprintf("notes/sp%d/n%d.md", writerID, i),
			Data: map[string]interface{}{"writer": writerID, "seq": i},
		}
		if err := em.Emit(context.Background(), ev); err != nil {
			fmt.Fprintf(os.Stderr, "writer %d emit failed at seq %d: %v\n", writerID, i, err)
			os.Exit(1)
		}
	}
	os.Exit(0)
}

// Test 2 (spec §11.1): Concurrent subprocess append.
//
// 16 subprocesses × 5,000 events each, all opening their own FD against
// the same daily file. This is the actual production layout (multiple
// sbdb processes from hooks + CLI sharing one events log).
func TestSubprocess_ConcurrentAppend(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in short mode")
	}
	tmp := t.TempDir()

	const subprocs = 16
	const perProc = 5000

	exe, err := testBinary()
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(subprocs)
	failures := make(chan error, subprocs)
	for w := 0; w < subprocs; w++ {
		go func(id int) {
			defer wg.Done()
			cmd := exec.Command(exe, "-test.run=^$") // run nothing; helper takes over via env
			cmd.Env = append(os.Environ(),
				helperEnv+"=1",
				"SBDB_TEST_APPEND_ROOT="+tmp,
				"SBDB_TEST_APPEND_COUNT="+strconv.Itoa(perProc),
				"SBDB_TEST_WRITER_ID="+strconv.Itoa(id),
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				failures <- fmt.Errorf("writer %d: %v: %s", id, err, out)
			}
		}(w)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if t.Failed() {
		return
	}

	got := readAllLineMapsForSubproc(t, tmp)
	require.Len(t, got, subprocs*perProc, "total line count")

	seen := make(map[int][]int, subprocs)
	for _, m := range got {
		w := int(m["data"].(map[string]interface{})["writer"].(float64))
		s := int(m["data"].(map[string]interface{})["seq"].(float64))
		seen[w] = append(seen[w], s)
	}
	require.Len(t, seen, subprocs, "every subprocess landed events")
	for w, seqs := range seen {
		require.Len(t, seqs, perProc, "writer %d", w)
		// per-writer order must be monotonic (kernel preserves single-FD ordering)
		for i, s := range seqs {
			require.Equal(t, i+1, s, "writer %d seq[%d]=%d", w, i, s)
		}
	}
}

func testBinary() (string, error) {
	// runtime.Caller gives this file; the test binary path is os.Args[0].
	// In `go test`, os.Args[0] is the compiled binary.
	exe := os.Args[0]
	if !filepath.IsAbs(exe) {
		abs, err := filepath.Abs(exe)
		if err != nil {
			return "", err
		}
		exe = abs
	}
	return exe, nil
}

func readAllLineMapsForSubproc(t *testing.T, root string) []map[string]interface{} {
	t.Helper()
	dir := filepath.Join(root, EventsDir)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var out []map[string]interface{}
	for _, ent := range entries {
		if !strings.HasSuffix(ent.Name(), ".jsonl") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, ent.Name()))
		require.NoError(t, err)
		for _, line := range strings.Split(string(data), "\n") {
			if line == "" {
				continue
			}
			var m map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(line), &m),
				"torn line in %s", ent.Name())
			out = append(out, m)
		}
	}
	return out
}

var _ = runtime.GOOS // keep import for future fs-type detection
