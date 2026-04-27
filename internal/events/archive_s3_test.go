package events

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeS3 implements S3Uploader entirely in memory. Used to drive S3Target
// through ArchiveExpired without an actual cloud dependency.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newFakeS3() *fakeS3 { return &fakeS3{objects: map[string][]byte{}} }

func (f *fakeS3) PutObject(ctx context.Context, bucket, key, localPath, md5 string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", err
	}
	f.objects[bucket+"/"+key] = data
	return "fake-etag", nil
}

func (f *fakeS3) HeadObject(ctx context.Context, bucket, key string) (int64, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.objects[bucket+"/"+key]
	if !ok {
		return 0, "", errors.New("not found")
	}
	return int64(len(data)), "fake-etag", nil
}

func TestArchiver_S3Target_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	r := NewBuiltinRegistry()

	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)

	app := NewAppender(tmp, 100_000)
	em := NewEmitter(app, r)
	for i := 0; i < 4; i++ {
		require.NoError(t, em.Emit(context.Background(), &Event{
			TS: feb.Add(time.Duration(i) * time.Minute), Type: "note.created",
			ID: "notes/old.md", SHA: fmt.Sprintf("sha%d", i),
		}))
	}
	require.NoError(t, app.Close())

	fake := newFakeS3()
	target := NewS3Target(tmp, "test-bucket", "events/", fake)
	arc := NewArchiver(tmp, target)
	arc.Now = func() time.Time { return now }

	sealed, err := arc.ArchiveExpired(context.Background())
	require.NoError(t, err)
	require.Len(t, sealed, 1)
	require.Equal(t, "s3", sealed[0].Target)
	require.Equal(t, "s3://test-bucket/events/2026-02.jsonl.gz", sealed[0].URI)

	// S3 has the object.
	_, _, err = fake.HeadObject(context.Background(), "test-bucket", "events/2026-02.jsonl.gz")
	require.NoError(t, err)

	// Pointer file written.
	pointerData, err := os.ReadFile(MonthPointerPath(tmp, 2026, 2))
	require.NoError(t, err)
	require.Contains(t, string(pointerData), "s3://test-bucket/events/2026-02.jsonl.gz")
	require.Contains(t, string(pointerData), "target: s3")

	// Local gz still kept (offline replay).
	_, err = os.Stat(MonthArchivePath(tmp, 2026, 2))
	require.NoError(t, err)
}

func TestArchiver_S3Target_Idempotent(t *testing.T) {
	tmp := t.TempDir()
	r := NewBuiltinRegistry()
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)

	app := NewAppender(tmp, 100_000)
	em := NewEmitter(app, r)
	require.NoError(t, em.Emit(context.Background(), &Event{
		TS: feb, Type: "note.created", ID: "notes/x.md",
	}))
	require.NoError(t, app.Close())

	fake := newFakeS3()
	target := NewS3Target(tmp, "b", "evt/", fake)
	arc := NewArchiver(tmp, target)
	arc.Now = func() time.Time { return now }

	sealed1, err := arc.ArchiveExpired(context.Background())
	require.NoError(t, err)
	require.Len(t, sealed1, 1)

	sealed2, err := arc.ArchiveExpired(context.Background())
	require.NoError(t, err)
	require.Empty(t, sealed2, "second run should be no-op")
}
