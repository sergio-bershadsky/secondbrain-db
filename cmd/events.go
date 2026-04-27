package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/events"
	schemapkg "github.com/sergio-bershadsky/secondbrain-db/internal/schema"
)

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Project events from git history",
	Long: `Events are not stored on disk; they are computed from git history on
demand. The repo's git log IS the event log — every CRUD operation produces
a commit, and the projection reads commit diffs to emit JSONL events.`,
}

var eventsEmitCmd = &cobra.Command{
	Use:   "emit <commit-from> [<commit-to>|latest]",
	Short: "Emit events for git commits in a range",
	Long: `Walk commits in <commit-from>..<commit-to> (chronological, oldest first)
and emit one JSONL event per file change under a known schema's docs_dir.

<commit-to> defaults to HEAD (or "latest"). <commit-from> may be any commit-ish
recognized by git: a sha, branch, tag, HEAD~N, @{1.week.ago}, etc.

Output is suitable for piping:

	sbdb events emit HEAD~50 | my-worker
	sbdb events emit v1.0.0 v1.1.0 | jq 'select(.type=="note.created")'

Files outside any schema's docs_dir produce no events.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runEventsEmit,
}

func init() {
	eventsCmd.AddCommand(eventsEmitCmd)
	rootCmd.AddCommand(eventsCmd)
}

func runEventsEmit(cmd *cobra.Command, args []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	from := args[0]
	to := ""
	if len(args) == 2 {
		to = args[1]
	}

	mapper, err := buildPathBucket(cfg.BasePath)
	if err != nil {
		return err
	}

	p := &events.Projector{
		Repo:         cfg.BasePath,
		PathToBucket: mapper,
	}
	return p.Emit(cmd.OutOrStdout(), from, to)
}

// buildPathBucket scans the project's schemas directory and returns a
// PathBucket that maps a repo-relative path to its bucket. The mapping is
// (longest matching docs_dir prefix) → schema bucket. Schemas without a
// docs_dir or with one that doesn't prefix the path are skipped.
//
// Schemas are read from the working tree at invocation time, not at the
// `to` commit. For ranges spanning schema renames or additions, re-run the
// projection from a checkout matching the historical schema layout.
func buildPathBucket(basePath string) (events.PathBucket, error) {
	schemasDir := filepath.Join(basePath, "schemas")
	entries, err := os.ReadDir(schemasDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No schemas → no events.
			return func(string) string { return "" }, nil
		}
		return nil, fmt.Errorf("reading schemas dir: %w", err)
	}

	type entry struct {
		dir, bucket string
	}
	var mappings []entry
	for _, ent := range entries {
		if ent.IsDir() || filepath.Ext(ent.Name()) != ".yaml" {
			continue
		}
		s, err := schemapkg.Load(filepath.Join(schemasDir, ent.Name()))
		if err != nil {
			return nil, fmt.Errorf("loading %s: %w", ent.Name(), err)
		}
		if s.DocsDir == "" {
			continue
		}
		bucket := s.Bucket
		if bucket == "" {
			bucket = s.Entity
		}
		mappings = append(mappings, entry{
			dir:    strings.TrimSuffix(s.DocsDir, "/") + "/",
			bucket: bucket,
		})
	}

	return func(path string) string {
		var bestPrefix, bestBucket string
		for _, m := range mappings {
			if strings.HasPrefix(path, m.dir) && len(m.dir) > len(bestPrefix) {
				bestPrefix = m.dir
				bestBucket = m.bucket
			}
		}
		return bestBucket
	}, nil
}
