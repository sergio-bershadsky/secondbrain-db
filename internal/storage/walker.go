package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// Doc is one parsed markdown document yielded by WalkDocs.
type Doc struct {
	Path        string         // absolute path to the .md file
	Frontmatter map[string]any // parsed YAML frontmatter (may be empty)
	Body        string         // markdown body after frontmatter
}

// WalkDocsToSlice walks docsDir recursively, parses every *.md file
// (skipping sidecar *.yaml and non-md files), and returns all Docs.
// Concurrency is bounded by SBDB_WALK_WORKERS (default GOMAXPROCS).
func WalkDocsToSlice(docsDir string) ([]Doc, error) {
	paths, err := collectMDPaths(docsDir)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}

	workers := WorkerCount()
	jobs := make(chan string, len(paths))
	for _, p := range paths {
		jobs <- p
	}
	close(jobs)

	results := make([]Doc, len(paths))
	errs := make([]error, len(paths))
	var wg sync.WaitGroup

	idx := 0
	idxMu := sync.Mutex{}
	nextIdx := func() int {
		idxMu.Lock()
		defer idxMu.Unlock()
		i := idx
		idx++
		return i
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				i := nextIdx()
				fm, body, perr := ParseMarkdown(p)
				if perr != nil {
					errs[i] = fmt.Errorf("%s: %w", p, perr)
					continue
				}
				results[i] = Doc{Path: p, Frontmatter: fm, Body: body}
			}
		}()
	}
	wg.Wait()

	for _, e := range errs {
		if e != nil {
			return nil, e
		}
	}

	out := results[:0]
	for _, d := range results {
		if d.Path != "" {
			out = append(out, d)
		}
	}
	return out, nil
}

func collectMDPaths(docsDir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(docsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == docsDir {
				return filepath.SkipAll
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".md") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", docsDir, err)
	}
	return paths, nil
}

// WorkerCount can be overridden by callers to control walker concurrency
// without setting the env var. Default reads SBDB_WALK_WORKERS env, then
// runtime.GOMAXPROCS(0).
var WorkerCount = defaultWorkerCount

func defaultWorkerCount() int {
	if v := os.Getenv("SBDB_WALK_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return runtime.GOMAXPROCS(0)
}
