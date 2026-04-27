package events

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestArchiver_GitTarget_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	r := NewBuiltinRegistry()

	// Pretend it's mid-April 2026; archive February 2026.
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)

	app := NewAppender(tmp, 100_000)
	em := NewEmitter(app, r)
	for i := 0; i < 5; i++ {
		require.NoError(t, em.Emit(context.Background(), &Event{
			TS:   feb.Add(time.Duration(i) * time.Minute),
			Type: "note.created",
			ID:   "notes/old.md",
			SHA:  fmt.Sprintf("sha%d", i),
		}))
	}
	require.NoError(t, app.Close())

	// Confirm the daily file landed.
	febPath := DailyPath(tmp, feb)
	stat, err := os.Stat(febPath)
	require.NoError(t, err)
	require.Greater(t, stat.Size(), int64(0))

	// Run archival as if today were 2026-04-24.
	target := NewGitTarget(tmp)
	arc := NewArchiver(tmp, target)
	arc.Now = func() time.Time { return now }
	sealed, err := arc.ArchiveExpired(context.Background())
	require.NoError(t, err)
	require.Len(t, sealed, 1)

	// Daily files for Feb gone.
	_, err = os.Stat(febPath)
	require.True(t, os.IsNotExist(err), "daily file should be removed after archive")

	// Archive present and verifiable.
	gzPath := MonthArchivePath(tmp, 2026, 2)
	gz, err := os.Stat(gzPath)
	require.NoError(t, err)
	require.Greater(t, gz.Size(), int64(0))

	// Content round-trip.
	events, err := ReadGzipFile(gzPath)
	require.NoError(t, err)
	require.Len(t, events, 5)

	// Year manifest exists.
	mf, err := os.ReadFile(YearManifestPath(tmp, 2026))
	require.NoError(t, err)
	require.Contains(t, string(mf), "2026-02")
}

func TestArchiver_LiveWindow_Untouched(t *testing.T) {
	tmp := t.TempDir()
	r := NewBuiltinRegistry()

	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	march := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC) // previous month → live

	app := NewAppender(tmp, 100_000)
	em := NewEmitter(app, r)
	require.NoError(t, em.Emit(context.Background(), &Event{
		TS:   march,
		Type: "note.created",
		ID:   "notes/march.md",
	}))
	require.NoError(t, app.Close())

	target := NewGitTarget(tmp)
	arc := NewArchiver(tmp, target)
	arc.Now = func() time.Time { return now }
	sealed, err := arc.ArchiveExpired(context.Background())
	require.NoError(t, err)
	require.Empty(t, sealed)

	// March daily file still present.
	_, err = os.Stat(DailyPath(tmp, march))
	require.NoError(t, err)
}

func TestArchiver_Idempotent(t *testing.T) {
	tmp := t.TempDir()
	r := NewBuiltinRegistry()
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)

	app := NewAppender(tmp, 100_000)
	em := NewEmitter(app, r)
	require.NoError(t, em.Emit(context.Background(), &Event{TS: feb, Type: "note.created", ID: "notes/x.md"}))
	require.NoError(t, app.Close())

	target := NewGitTarget(tmp)
	arc := NewArchiver(tmp, target)
	arc.Now = func() time.Time { return now }
	sealed1, err := arc.ArchiveExpired(context.Background())
	require.NoError(t, err)
	require.Len(t, sealed1, 1)

	// Re-run: idempotent (already archived).
	sealed2, err := arc.ArchiveExpired(context.Background())
	require.NoError(t, err)
	require.Empty(t, sealed2)
}

// TestArchiver_SettlePeriodGate verifies the settle period is enforced as an
// additional gate on top of the live-window check. In default config the
// settle (7 days) always lapses before the live window expires, so we
// configure an aggressive long settle to demonstrate the gate works.
func TestArchiver_SettlePeriodGate(t *testing.T) {
	tmp := t.TempDir()
	r := NewBuiltinRegistry()
	// April 5 — Feb is outside live window (live = March + April).
	now := time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC)

	app := NewAppender(tmp, 100_000)
	em := NewEmitter(app, r)
	require.NoError(t, em.Emit(context.Background(), &Event{TS: feb, Type: "note.created", ID: "notes/x.md"}))
	require.NoError(t, app.Close())

	target := NewGitTarget(tmp)
	arc := NewArchiver(tmp, target)
	arc.Now = func() time.Time { return now }
	// 60-day settle pushes Feb's deadline to May 1. April 5 < May 1.
	arc.SettleDays = 60

	sealed, err := arc.ArchiveExpired(context.Background())
	require.NoError(t, err)
	require.Empty(t, sealed, "should not archive before settle deadline")

	// Default 7-day settle would archive: deadline is March 8, today is April 5.
	arc.SettleDays = 7
	sealed, err = arc.ArchiveExpired(context.Background())
	require.NoError(t, err)
	require.Len(t, sealed, 1)

	files := readDir(t, filepath.Join(tmp, EventsDir))
	for _, f := range files {
		require.NotContains(t, f, "2026-02-", "feb dailies removed: %s", f)
	}
}
