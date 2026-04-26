package events

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Appender writes events to the daily file under a given project root.
//
// Concurrency model (spec §11): Append uses POSIX O_APPEND and a single
// write() syscall per event line ≤ 4 KiB. No file locking is required;
// concurrent writers (across goroutines AND processes) are race-free by
// kernel guarantee.
//
// Appender holds a per-day file descriptor cache to avoid open/close on
// every append. Crossing midnight UTC rolls the cached fd to the new day.
type Appender struct {
	root          string
	rotationLines int
	maxLineBytes  int

	mu          sync.Mutex
	currentFile *os.File
	currentName string
	currentDate time.Time // UTC date of currentFile (truncated to day)
	lineCount   int       // approximate, used for rotation
	rotationN   int       // current slice (0 = base file)
}

// NewAppender returns an Appender rooted at the given project directory.
func NewAppender(root string, rotationLines int) *Appender {
	if rotationLines <= 0 {
		rotationLines = 5000
	}
	return &Appender{
		root:          root,
		rotationLines: rotationLines,
		maxLineBytes:  MaxLineBytes,
	}
}

// Close releases the cached file descriptor.
func (a *Appender) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.currentFile != nil {
		err := a.currentFile.Close()
		a.currentFile = nil
		return err
	}
	return nil
}

// Append serializes and writes one event. Returns ErrLineTooLarge if the
// event would exceed the size cap; the file is unchanged in that case.
func (a *Appender) Append(ctx context.Context, e *Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	line, err := e.MarshalLine()
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.ensureFileLocked(e.TS); err != nil {
		return err
	}

	// Single write() syscall. O_APPEND atomicity holds for writes ≤ PIPE_BUF
	// on regular files; we cap at 4 KiB which is well within that bound.
	n, err := a.currentFile.Write(line)
	if err != nil {
		return err
	}
	if n != len(line) {
		// Should not happen for small writes on local POSIX FS, but be loud
		// about it if it does — see spec §11 test 4.
		return fmt.Errorf("short write: wrote %d of %d bytes", n, len(line))
	}
	a.lineCount++
	return nil
}

// ensureFileLocked makes sure currentFile points at the right daily file
// for time t. If we've crossed midnight UTC or the line cap is hit, we roll.
// Caller must hold a.mu.
func (a *Appender) ensureFileLocked(t time.Time) error {
	t = t.UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)

	// New day → close current and reset counters.
	if a.currentFile != nil && !a.currentDate.Equal(day) {
		_ = a.currentFile.Close()
		a.currentFile = nil
		a.lineCount = 0
		a.rotationN = 0
	}

	// Rotation: if we crossed line cap on the current slice, advance.
	if a.currentFile != nil && a.lineCount >= a.rotationLines {
		_ = a.currentFile.Close()
		a.currentFile = nil
		a.rotationN++
		a.lineCount = 0
	}

	if a.currentFile != nil {
		return nil
	}

	// Determine which slice to open. If the base file exists and is at/over
	// the cap, find the highest existing rotation; if none under the cap,
	// open a new rotation. Cheaper path: just compute by reading filesystem.
	a.currentDate = day

	// On startup we may not know how full the base file is. Scan once.
	if a.rotationN == 0 {
		n, lines, err := a.discoverRotationLocked(day)
		if err != nil {
			return err
		}
		a.rotationN = n
		a.lineCount = lines
		// If the discovered slice is full, advance.
		if a.lineCount >= a.rotationLines {
			a.rotationN++
			a.lineCount = 0
		}
	}

	a.currentName = RotationFileName(day, a.rotationN)
	path := filepath.Join(a.root, EventsDir, a.currentName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	// O_APPEND | O_WRONLY | O_CREATE — never O_TRUNC.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	a.currentFile = f
	return nil
}

// discoverRotationLocked scans the events directory to find the highest
// existing rotation slice for `day` and counts its lines.
func (a *Appender) discoverRotationLocked(day time.Time) (int, int, error) {
	dir := filepath.Join(a.root, EventsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	prefix := DailyFileName(day) // "YYYY-MM-DD.jsonl"
	prefixBase := prefix[:len(prefix)-len(".jsonl")]
	highest := -1
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if name == prefix && highest < 0 {
			highest = 0
			continue
		}
		// rotation: "YYYY-MM-DD.NNN.jsonl"
		_, _, _, slice, ok := ParseDailyName(name)
		if !ok {
			continue
		}
		if !hasPrefix(name, prefixBase) {
			continue
		}
		if slice > highest {
			highest = slice
		}
	}
	if highest < 0 {
		return 0, 0, nil
	}
	// count lines in that slice
	path := filepath.Join(dir, RotationFileName(day, highest))
	lines, err := countLinesIn(path)
	if err != nil {
		return 0, 0, err
	}
	return highest, lines, nil
}

func hasPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}

func countLinesIn(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), MaxLineBytes+128)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}
