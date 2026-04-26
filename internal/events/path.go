package events

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// EventsDir is the on-disk root for the events log, relative to project root.
const EventsDir = ".sbdb/events"

// ArchiveDir is the on-disk archive directory, relative to project root.
const ArchiveDir = ".sbdb/events/archive"

// DailyFileName returns the basename for a daily file at the given UTC date.
// Example: time(2026, 4, 24) -> "2026-04-24.jsonl".
func DailyFileName(t time.Time) string {
	t = t.UTC()
	return fmt.Sprintf("%04d-%02d-%02d.jsonl", t.Year(), int(t.Month()), t.Day())
}

// DailyPath returns the absolute path to the daily file for time t under root.
func DailyPath(root string, t time.Time) string {
	return filepath.Join(root, EventsDir, DailyFileName(t))
}

// RotationFileName returns "YYYY-MM-DD.NNN.jsonl" for the n-th rotation slice
// of a daily file. n=0 is the base file (no NNN suffix); n>=1 is rotation.
func RotationFileName(t time.Time, n int) string {
	t = t.UTC()
	if n == 0 {
		return DailyFileName(t)
	}
	return fmt.Sprintf("%04d-%02d-%02d.%03d.jsonl", t.Year(), int(t.Month()), t.Day(), n)
}

// MonthArchiveName returns "YYYY-MM.jsonl.gz" for archived monthly bundles.
func MonthArchiveName(year, month int) string {
	return fmt.Sprintf("%04d-%02d.jsonl.gz", year, month)
}

// MonthArchivePath returns the absolute path to a month's gzipped archive.
func MonthArchivePath(root string, year, month int) string {
	return filepath.Join(root, ArchiveDir, MonthArchiveName(year, month))
}

// MonthPointerPath returns the absolute path to a month's S3 pointer file.
func MonthPointerPath(root string, year, month int) string {
	return filepath.Join(root, ArchiveDir, fmt.Sprintf("%04d-%02d.pointer.yaml", year, month))
}

// YearManifestPath returns the absolute path to a year's manifest YAML.
func YearManifestPath(root string, year int) string {
	return filepath.Join(root, ArchiveDir, fmt.Sprintf("%04d.MANIFEST.yaml", year))
}

// IsLiveMonth reports whether (year, month) falls in the 2-month live window
// at time `now`. Returns true for the current month and the immediately
// previous month.
//
// Spec §7.5: at any moment, only the current month and the immediately
// previous month exist as live .jsonl files.
func IsLiveMonth(year, month int, now time.Time) bool {
	now = now.UTC()
	curY, curM := now.Year(), int(now.Month())
	if year == curY && month == curM {
		return true
	}
	prevY, prevM := curY, curM-1
	if prevM == 0 {
		prevM = 12
		prevY--
	}
	return year == prevY && month == prevM
}

// MonthSettleDeadline returns the moment at which an expired month becomes
// eligible for archival. Spec §7.7: end-of-month + settle days.
func MonthSettleDeadline(year, month, settleDays int) time.Time {
	// last day of the month
	t := time.Date(year, time.Month(month)+1, 1, 0, 0, 0, 0, time.UTC)
	return t.AddDate(0, 0, settleDays)
}

// ParseDailyName parses "YYYY-MM-DD.jsonl" or "YYYY-MM-DD.NNN.jsonl" and
// returns (year, month, day, slice, ok). slice=0 for the base file.
func ParseDailyName(name string) (year, month, day, slice int, ok bool) {
	base := strings.TrimSuffix(name, ".jsonl")
	if base == name {
		return
	}
	parts := strings.Split(base, ".")
	// parts[0] is YYYY-MM-DD
	if len(parts) < 1 {
		return
	}
	dateParts := strings.Split(parts[0], "-")
	if len(dateParts) != 3 {
		return
	}
	y, m, d, err := parseYMD(dateParts[0], dateParts[1], dateParts[2])
	if err != nil {
		return
	}
	year, month, day = y, m, d
	if len(parts) == 2 {
		// rotation slice
		var n int
		if _, err := fmt.Sscanf(parts[1], "%03d", &n); err != nil {
			return
		}
		slice = n
	}
	ok = true
	return
}

func parseYMD(y, m, d string) (int, int, int, error) {
	var year, month, day int
	var err error
	if year, err = atoiPad(y, 4); err != nil {
		return 0, 0, 0, err
	}
	if month, err = atoiPad(m, 2); err != nil {
		return 0, 0, 0, err
	}
	if day, err = atoiPad(d, 2); err != nil {
		return 0, 0, 0, err
	}
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return 0, 0, 0, fmt.Errorf("invalid date components")
	}
	return year, month, day, nil
}

func atoiPad(s string, expectedLen int) (int, error) {
	if len(s) != expectedLen {
		return 0, fmt.Errorf("expected %d digits, got %q", expectedLen, s)
	}
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("non-digit in %q", s)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

// SortedDailyFiles returns daily file basenames in lex order. Names not
// matching the daily pattern are skipped.
func SortedDailyFiles(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if _, _, _, _, ok := ParseDailyName(n); ok {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}
