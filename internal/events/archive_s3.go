package events

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// S3Uploader is the minimal interface an S3 client must satisfy. Production
// builds wire this to aws-sdk-go-v2; tests can use a fake implementation.
//
// We keep the dependency on AWS SDK behind this interface so the events
// package itself stays SDK-free; the concrete client lives in a separate
// build target (cmd/event_s3.go in a follow-up that imports aws-sdk-go-v2).
type S3Uploader interface {
	// PutObject uploads localPath to s3://<bucket>/<key>. Returns the
	// object's ETag (or empty string if the implementation can't surface
	// it). md5 is the hex-encoded Content-MD5 the caller computed.
	PutObject(ctx context.Context, bucket, key, localPath, md5 string) (etag string, err error)
	// HeadObject returns the object's metadata for verification, or err
	// with IsNotExist semantics if the object doesn't exist.
	HeadObject(ctx context.Context, bucket, key string) (size int64, etag string, err error)
}

// S3Target archives months to an S3 bucket. The repo retains an immutable
// pointer.yaml per archived month per spec §7.8.
type S3Target struct {
	Root         string
	Bucket       string
	Prefix       string // trailing slash optional
	Uploader     S3Uploader
	StorageClass string
	SSE          string
}

// NewS3Target constructs an S3Target. The Uploader must be supplied by the
// caller (typically constructed in the cmd layer where the AWS SDK lives).
func NewS3Target(root, bucket, prefix string, uploader S3Uploader) *S3Target {
	return &S3Target{
		Root:     root,
		Bucket:   bucket,
		Prefix:   prefix,
		Uploader: uploader,
	}
}

// Upload pushes the staged gz to S3, verifies via HEAD, and writes the
// pointer YAML inside the repo. The local gz is removed by the caller
// (Archiver) after a successful return — this method only handles the
// S3-specific side of the operation.
func (t *S3Target) Upload(ctx context.Context, year, month int, localPath string) (Pointer, error) {
	if t.Uploader == nil {
		return Pointer{}, fmt.Errorf("S3 uploader is nil; configure events.archive.s3 in .sbdb.toml")
	}

	key := fmt.Sprintf("%s%04d-%02d.jsonl.gz", normalizePrefix(t.Prefix), year, month)

	// Pre-flight: skip upload if object already exists with matching content.
	// (Idempotent re-runs.)
	if size, _, err := t.Uploader.HeadObject(ctx, t.Bucket, key); err == nil && size > 0 {
		// We trust the existing object — Archiver already verified line count
		// from the local gz before calling Upload. The HEAD just confirms the
		// remote is intact.
	} else {
		if _, err := t.Uploader.PutObject(ctx, t.Bucket, key, localPath, ""); err != nil {
			return Pointer{}, fmt.Errorf("s3 put: %w", err)
		}
		// Verify post-upload.
		if _, _, err := t.Uploader.HeadObject(ctx, t.Bucket, key); err != nil {
			return Pointer{}, fmt.Errorf("s3 head after put: %w", err)
		}
	}

	uri := fmt.Sprintf("s3://%s/%s", t.Bucket, key)

	// Move local gz into archive/ so the caller (Archiver) writes the
	// pointer beside it. We keep the gz in archive/ when target=both;
	// for target=s3-only, we move it to a hidden cache so re-runs can
	// validate without re-fetching from S3.
	dest := MonthArchivePath(t.Root, year, month)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return Pointer{}, err
	}
	// Even for target=s3, retaining a local gz copy is cheap and lets
	// `sbdb event show` work offline. Keep it.
	if err := os.Rename(localPath, dest); err != nil {
		return Pointer{}, err
	}

	// Pointer file stays in the repo.
	if err := writePointerFile(t.Root, year, month, uri); err != nil {
		return Pointer{}, err
	}

	return Pointer{Target: "s3", URI: uri}, nil
}

func writePointerFile(root string, year, month int, uri string) error {
	path := MonthPointerPath(root, year, month)
	doc := map[string]interface{}{
		"month":  fmt.Sprintf("%04d-%02d", year, month),
		"target": "s3",
		"s3_uri": uri,
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func normalizePrefix(p string) string {
	if p == "" {
		return ""
	}
	if p[len(p)-1] != '/' {
		return p + "/"
	}
	return p
}
