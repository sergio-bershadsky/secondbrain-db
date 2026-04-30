package events

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// PathBucket maps a repo-relative POSIX path to an event bucket. A non-empty
// return value means the path belongs to a known schema and should produce
// events; "" means skip this path entirely.
//
// Implementations typically hold a list of (docs_dir, bucket) pairs and
// pick the longest-prefix match.
type PathBucket func(path string) string

// Projector projects git history onto the event stream. It shells out to
// `git log --raw` (which exists in every git-equipped environment, including
// CI) and parses the output line-by-line, writing one Event per changed
// file under a known bucket.
type Projector struct {
	// Repo is the path to the git repository root.
	Repo string

	// PathToBucket decides which paths produce events.
	PathToBucket PathBucket
}

// Emit walks commits in the range `from..to` (chronological order, oldest
// first) and writes one JSONL event per change to w. `to` may be the empty
// string or "latest", in which case it defaults to HEAD. `from` is required
// and may be any commit-ish (sha, branch, tag, HEAD~N, @{1.week.ago}, etc.).
//
// One commit produces N events where N is the number of changed files
// under a bucket-mapped docs_dir. Initial-commit handling: a commit with
// no parent is walked as if every file is `A` (new). Merge commits use
// their first parent (matches git's default `git log` behavior).
func (p *Projector) Emit(w io.Writer, from, to string) error {
	if to == "" || to == "latest" {
		to = "HEAD"
	}
	rangeSpec := from + ".." + to

	// --raw: include diff metadata with blob hashes
	// --no-renames: surface renames as paired D+A so the wire format stays
	//   structural (consumers collapse if they care)
	// --reverse: oldest-first, the natural order workers want
	// --format: a header line per commit we can recognize
	// -z: NUL-terminated path fields, robust against whitespace/quoting
	cmd := exec.Command(
		"git",
		"-C", p.Repo,
		"log",
		"--reverse",
		"--raw",
		"--no-renames",
		"--no-merges",
		"-z",
		"--format=COMMIT%x00%H%x00%ct%x00%aE",
		rangeSpec,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}

	if err := p.parseLog(stdout, w); err != nil {
		_ = cmd.Wait()
		return err
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git log failed: %w (stderr: %s)", err, stderr.String())
	}
	return nil
}

// parseLog reads the NUL-delimited stream from `git log -z --raw` and
// writes one event per change row.
//
// The grammar (with NUL written as \0):
//
//	COMMIT\0<sha>\0<unix-ts>\0<author-email>\0
//	(for each changed file:)
//	  :<old-mode> <new-mode> <old-blob> <new-blob> <status>\0<path>\0
//	(repeated; another COMMIT\0... starts the next commit)
//
// We stream the whole stdout into a NUL-split scanner and walk the tokens.
func (p *Projector) parseLog(r io.Reader, w io.Writer) error {
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 64*1024), 4*1024*1024)
	scan.Split(splitNUL)

	type commitCtx struct {
		sha    string
		ts     time.Time
		author string
	}
	var cur commitCtx

	for scan.Scan() {
		// git inserts a literal "\n" between the --format header and the
		// first --raw row of each commit, so diff-row tokens arrive with
		// a leading newline. Strip it before classification.
		tok := strings.TrimLeft(scan.Text(), "\n")
		if tok == "COMMIT" {
			// Next three tokens are commit metadata.
			if !scan.Scan() {
				return fmt.Errorf("unexpected end after COMMIT marker")
			}
			cur.sha = scan.Text()
			if !scan.Scan() {
				return fmt.Errorf("unexpected end after commit sha")
			}
			tsRaw := scan.Text()
			tsUnix, err := strconv.ParseInt(tsRaw, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid commit ts %q: %w", tsRaw, err)
			}
			cur.ts = time.Unix(tsUnix, 0).UTC()
			if !scan.Scan() {
				return fmt.Errorf("unexpected end after commit ts")
			}
			cur.author = scan.Text()
			continue
		}

		// A diff row token. Format: ":100644 100644 oldblob newblob M"
		// followed by a separate path token. The leading ":" is on the
		// raw row; bufio.Scanner with NUL split gives us the row up to
		// (but not including) the NUL, then the path as the next token.
		if !strings.HasPrefix(tok, ":") {
			// Skip blank or unexpected tokens (e.g. trailing newlines git
			// inserts between commits before the next COMMIT marker).
			continue
		}
		fields := strings.Fields(tok)
		if len(fields) < 5 {
			return fmt.Errorf("malformed diff row: %q", tok)
		}
		oldBlob := fields[2]
		newBlob := fields[3]
		status := fields[4][0]

		// Path follows as a separate NUL-terminated token.
		if !scan.Scan() {
			return fmt.Errorf("missing path after diff row")
		}
		path := scan.Text()
		if path == "" {
			continue
		}

		bucket := p.PathToBucket(path)
		if bucket == "" {
			continue // not under any schema's docs_dir
		}
		verb := VerbForStatus(status)
		if verb == "" {
			continue
		}

		ev := &Event{
			TS:    cur.ts,
			Type:  bucket + "." + verb,
			ID:    path,
			Op:    cur.sha,
			Actor: defaultActor(cur.author),
		}
		// Zero-blob means absent (created → no prev; deleted → no sha).
		if !isZeroBlob(newBlob) {
			ev.SHA = newBlob
		}
		if !isZeroBlob(oldBlob) {
			ev.Prev = oldBlob
		}

		line, err := ev.MarshalLine()
		if err != nil {
			return fmt.Errorf("marshal event for %s: %w", path, err)
		}
		if _, err := w.Write(line); err != nil {
			return err
		}
	}
	return scan.Err()
}

// splitNUL is a bufio.Scanner SplitFunc that returns tokens delimited by
// the NUL byte (matching git's -z output format).
func splitNUL(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// isZeroBlob reports whether a blob hash is the all-zeros sentinel that
// git uses to mean "absent" (e.g. on the pre-image side of an `A` row).
func isZeroBlob(s string) bool {
	for _, c := range s {
		if c != '0' {
			return false
		}
	}
	return s != ""
}

func defaultActor(email string) string {
	if email == "" {
		return "git"
	}
	return email
}
