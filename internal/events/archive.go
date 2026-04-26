package events

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ArchiveTarget abstracts where a sealed monthly bundle is uploaded.
// Implementations: gitTarget (in-repo), s3Target (S3-compatible).
type ArchiveTarget interface {
	// Upload writes the gz file at localPath to the destination identified
	// by month (YYYY-MM). Returns a Pointer describing the result for
	// inclusion in the year manifest.
	Upload(ctx context.Context, year, month int, localPath string) (Pointer, error)
}

// Pointer captures everything needed to verify and locate an archived month.
// Stored in <year>.MANIFEST.yaml entries and (for S3) per-month .pointer.yaml.
type Pointer struct {
	Month     string    `yaml:"month"` // "YYYY-MM"
	LineCount int       `yaml:"line_count"`
	SHA256    string    `yaml:"sha256"`    // of decompressed bytes
	GzSHA256  string    `yaml:"gz_sha256"` // of gz blob
	GzBytes   int64     `yaml:"gz_bytes"`
	Target    string    `yaml:"target"`        // "git" or "s3"
	URI       string    `yaml:"uri,omitempty"` // s3://... or relative path
	SealedAt  time.Time `yaml:"sealed_at"`
	SealedBy  string    `yaml:"sealed_by,omitempty"` // "sbdb@<version>"
}

// YearManifest is the per-year roll-up of archived months.
type YearManifest struct {
	Year   int                 `yaml:"year"`
	Months map[string]*Pointer `yaml:"months"`
}

// Archiver encapsulates the doctor-fix archival logic. It is created with
// a project root and target; calling ArchiveExpired walks live daily files
// and seals everything outside the live window.
type Archiver struct {
	Root       string
	Target     ArchiveTarget
	GzipLevel  int
	SettleDays int
	MaxBytes   int64
	SealedBy   string
	Now        func() time.Time
}

// NewArchiver returns an Archiver with sensible defaults.
func NewArchiver(root string, target ArchiveTarget) *Archiver {
	return &Archiver{
		Root:       root,
		Target:     target,
		GzipLevel:  9,
		SettleDays: 7,
		MaxBytes:   1 << 30,
		SealedBy:   "sbdb",
		Now:        func() time.Time { return time.Now().UTC() },
	}
}

// ArchiveExpired walks the live events dir, groups daily files by (year,
// month), and archives every month older than the live window past its
// settle deadline. Idempotent: re-running on already-archived months is a
// no-op.
func (a *Archiver) ArchiveExpired(ctx context.Context) ([]Pointer, error) {
	dir := filepath.Join(a.Root, EventsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Group by (year, month).
	grouped := map[[2]int][]string{}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		y, m, _, _, ok := ParseDailyName(name)
		if !ok {
			continue
		}
		key := [2]int{y, m}
		grouped[key] = append(grouped[key], name)
	}

	now := a.Now()
	var sealed []Pointer
	keys := make([][2]int, 0, len(grouped))
	for k := range grouped {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})

	for _, k := range keys {
		year, month := k[0], k[1]
		if IsLiveMonth(year, month, now) {
			continue
		}
		if now.Before(MonthSettleDeadline(year, month, a.SettleDays)) {
			continue
		}
		// Skip if already archived (idempotent).
		if _, err := os.Stat(MonthArchivePath(a.Root, year, month)); err == nil {
			// Pointer file may also exist for s3.
			continue
		} else if _, err := os.Stat(MonthPointerPath(a.Root, year, month)); err == nil {
			continue
		}

		ptr, err := a.archiveMonth(ctx, year, month, grouped[k])
		if err != nil {
			return sealed, fmt.Errorf("archive %04d-%02d: %w", year, month, err)
		}
		sealed = append(sealed, ptr)
	}
	return sealed, nil
}

// archiveMonth concatenates inputs in lex order, gzips, verifies, uploads
// via Target, writes year manifest, removes daily files. Atomic in spirit:
// if any step fails before removal, dailies stay intact.
func (a *Archiver) archiveMonth(ctx context.Context, year, month int, inputs []string) (Pointer, error) {
	sort.Strings(inputs)

	dir := filepath.Join(a.Root, EventsDir)

	// Concatenate to a tempfile and gzip in one pass.
	tmpGz, err := os.CreateTemp(dir, fmt.Sprintf(".archive-%04d-%02d-*.jsonl.gz", year, month))
	if err != nil {
		return Pointer{}, err
	}
	tmpPath := tmpGz.Name()
	defer func() {
		_ = os.Remove(tmpPath) // safe if already renamed/uploaded
	}()

	hasher := sha256.New()
	gzHasher := sha256.New()
	gzWriter, err := gzip.NewWriterLevel(io.MultiWriter(tmpGz, gzHasher), a.GzipLevel)
	if err != nil {
		_ = tmpGz.Close()
		return Pointer{}, err
	}

	lineCount := 0
	for _, name := range inputs {
		path := filepath.Join(dir, name)
		f, err := os.Open(path)
		if err != nil {
			_ = gzWriter.Close()
			_ = tmpGz.Close()
			return Pointer{}, err
		}
		// Tee: hash decompressed contents while writing through gz.
		teeReader := io.TeeReader(f, hasher)
		buf := make([]byte, 64*1024)
		for {
			n, rerr := teeReader.Read(buf)
			if n > 0 {
				if _, werr := gzWriter.Write(buf[:n]); werr != nil {
					_ = f.Close()
					_ = gzWriter.Close()
					_ = tmpGz.Close()
					return Pointer{}, werr
				}
				// count newlines
				for _, b := range buf[:n] {
					if b == '\n' {
						lineCount++
					}
				}
			}
			if rerr != nil {
				if rerr != io.EOF {
					_ = f.Close()
					_ = gzWriter.Close()
					_ = tmpGz.Close()
					return Pointer{}, rerr
				}
				break
			}
		}
		_ = f.Close()
	}
	if err := gzWriter.Close(); err != nil {
		_ = tmpGz.Close()
		return Pointer{}, err
	}
	if err := tmpGz.Sync(); err != nil {
		_ = tmpGz.Close()
		return Pointer{}, err
	}
	gzSize, err := tmpGz.Seek(0, io.SeekEnd)
	if err != nil {
		_ = tmpGz.Close()
		return Pointer{}, err
	}
	_ = tmpGz.Close()

	if a.MaxBytes > 0 && gzSize > a.MaxBytes {
		return Pointer{}, fmt.Errorf("archive size %d exceeds max %d (raise events.archive.max_archive_bytes if intentional)",
			gzSize, a.MaxBytes)
	}

	// Verify line count via decompression.
	gzLines, err := CountGzLines(tmpPath)
	if err != nil {
		return Pointer{}, fmt.Errorf("verify gz line count: %w", err)
	}
	if gzLines != lineCount {
		return Pointer{}, fmt.Errorf("line count mismatch: input=%d gz=%d", lineCount, gzLines)
	}

	ptr := Pointer{
		Month:     fmt.Sprintf("%04d-%02d", year, month),
		LineCount: lineCount,
		SHA256:    hex.EncodeToString(hasher.Sum(nil)),
		GzSHA256:  hex.EncodeToString(gzHasher.Sum(nil)),
		GzBytes:   gzSize,
		SealedAt:  a.Now(),
		SealedBy:  a.SealedBy,
	}

	// Hand off to the target. For git, this renames into place; for s3,
	// uploads + writes pointer file.
	uploaded, err := a.Target.Upload(ctx, year, month, tmpPath)
	if err != nil {
		return Pointer{}, err
	}
	// Merge fields from upload (target, uri).
	ptr.Target = uploaded.Target
	if uploaded.URI != "" {
		ptr.URI = uploaded.URI
	}

	// Update year manifest.
	if err := a.updateYearManifest(year, ptr); err != nil {
		return Pointer{}, err
	}

	// Remove daily files (only after successful archival + manifest write).
	for _, name := range inputs {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return Pointer{}, fmt.Errorf("remove %s: %w", name, err)
		}
	}
	return ptr, nil
}

// updateYearManifest writes an atomic update to <year>.MANIFEST.yaml.
func (a *Archiver) updateYearManifest(year int, ptr Pointer) error {
	path := YearManifestPath(a.Root, year)
	var ym YearManifest
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &ym); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if ym.Months == nil {
		ym.Months = map[string]*Pointer{}
	}
	ym.Year = year
	p := ptr
	ym.Months[ptr.Month] = &p

	data, err := yaml.Marshal(&ym)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// GitTarget archives into the in-repo `archive/` directory.
type GitTarget struct {
	Root string
}

// NewGitTarget returns a GitTarget rooted at the project directory.
func NewGitTarget(root string) *GitTarget { return &GitTarget{Root: root} }

// Upload renames the staging gz into archive/YYYY-MM.jsonl.gz.
func (t *GitTarget) Upload(ctx context.Context, year, month int, localPath string) (Pointer, error) {
	dest := MonthArchivePath(t.Root, year, month)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return Pointer{}, err
	}
	if err := os.Rename(localPath, dest); err != nil {
		return Pointer{}, err
	}
	rel := strings.TrimPrefix(dest, t.Root+string(os.PathSeparator))
	return Pointer{Target: "git", URI: rel}, nil
}
