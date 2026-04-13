package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/output"
)

var initTemplate string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new secondbrain-db project",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().StringVar(&initTemplate, "template", "notes", "template: notes, blog, adr, discussion, task")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	format := outputFormat(cfg)
	basePath := cfg.BasePath

	// Create directories
	dirs := []string{
		filepath.Join(basePath, "schemas"),
		filepath.Join(basePath, "docs"),
		filepath.Join(basePath, "data"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", d, err)
		}
	}

	// Write schema
	schemaContent := templateSchema(initTemplate)
	schemaPath := filepath.Join(basePath, "schemas", initTemplate+".yaml")
	if err := os.WriteFile(schemaPath, []byte(schemaContent), 0o644); err != nil {
		return err
	}

	// Write .sbdb.toml
	tomlContent := fmt.Sprintf(`schema_dir = "./schemas"
base_path = "."
default_schema = %q

[output]
format = "auto"

[integrity]
key_source = "env"
`, initTemplate)

	tomlPath := filepath.Join(basePath, ".sbdb.toml")
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o644); err != nil {
		return err
	}

	return output.PrintData(format, map[string]any{
		"action":   "init",
		"template": initTemplate,
		"schema":   schemaPath,
		"config":   tomlPath,
	})
}

func templateSchema(name string) string {
	switch name {
	case "blog":
		return `version: 1
entity: posts
docs_dir: docs/posts
filename: "{slug}.md"
records_dir: data/posts
partition: none
id_field: slug
integrity: strict

fields:
  slug:      { type: string, required: true }
  created:   { type: date, required: true }
  published: { type: bool, default: false }
  draft:     { type: bool, default: true }
  tags:      { type: list, items: { type: string } }

virtuals:
  title:
    returns: string
    source: |
      def compute(content, fields):
          for line in content.splitlines():
              if line.startswith("# "):
                  return line.removeprefix("# ").strip()
          return fields["slug"]
  reading_time:
    returns: int
    source: |
      def compute(content, fields):
          words = len(content.split())
          return max(1, words // 200)
`
	case "adr":
		return `version: 1
entity: adrs
docs_dir: docs/adrs
filename: "adr-{number}-{slug}.md"
records_dir: data/adrs
partition: none
id_field: number
integrity: strict

fields:
  number:   { type: int, required: true }
  slug:     { type: string, required: true }
  category: { type: string, required: true }
  created:  { type: date, required: true }
  author:   { type: string, required: true }

virtuals:
  title:
    returns: string
    source: |
      def compute(content, fields):
          for line in content.splitlines():
              if line.startswith("# "):
                  return line.removeprefix("# ").strip()
          return "ADR-" + str(fields["number"])
  status:
    returns: string
    source: |
      def compute(content, fields):
          for line in content.splitlines():
              if "**Status:**" in line:
                  return line.split("**Status:**")[1].strip().lower()
          return "draft"
`
	case "discussion":
		return `version: 1
entity: discussions
docs_dir: docs/discussions
filename: "{id}.md"
records_dir: data/discussions
partition: monthly
date_field: date
id_field: id
integrity: strict

fields:
  id:           { type: string, required: true }
  date:         { type: date, required: true }
  topic:        { type: string, required: true }
  participants: { type: list, items: { type: string } }
  source:       { type: enum, values: [manual, meeting, slack, fireflies, otter, telegram, email], default: manual }
  meeting_id:   { type: string }
  tags:         { type: list, items: { type: string } }
  status:       { type: enum, values: [documented, pending-review, archived], default: documented }

virtuals:
  title:
    returns: string
    source: |
      def compute(content, fields):
          for line in content.splitlines():
              if line.startswith("# "):
                  return line.removeprefix("# ").strip()
          return fields["topic"]
`
	case "task":
		return `version: 1
entity: tasks
docs_dir: docs/tasks
filename: "{id}.md"
records_dir: data/tasks
partition: none
id_field: id
integrity: strict

fields:
  id:             { type: string, required: true }
  number:         { type: int, required: true }
  title:          { type: string, required: true }
  status:         { type: enum, values: [todo, in_progress, blocked, done, canceled], default: todo }
  priority:       { type: enum, values: [low, medium, high, critical], default: medium }
  created:        { type: date, required: true }
  due_date:       { type: date }
  completed_date: { type: date }
  assignee:       { type: string }
  tags:           { type: list, items: { type: string } }

virtuals:
  title_from_content:
    returns: string
    source: |
      def compute(content, fields):
          for line in content.splitlines():
              if line.startswith("# "):
                  return line.removeprefix("# ").strip()
          return fields["title"]
  checklist_progress:
    returns: string
    source: |
      def compute(content, fields):
          done = 0
          total = 0
          for line in content.splitlines():
              if line.strip().startswith("- [x]"):
                  done += 1
                  total += 1
              elif line.strip().startswith("- [ ]"):
                  total += 1
          if total == 0:
              return "0/0"
          return str(done) + "/" + str(total)
`
	default: // notes
		return `version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
records_dir: data/notes
partition: none
id_field: id
integrity: strict

fields:
  id:      { type: string, required: true }
  created: { type: date, required: true }
  status:  { type: enum, values: [active, archived], default: active }
  tags:    { type: list, items: { type: string } }
  sources:
    type: list
    items:
      type: object
      fields:
        type: { type: string, required: true }
        link: { type: string, required: true }

virtuals:
  title:
    returns: string
    source: |
      def compute(content, fields):
          for line in content.splitlines():
              if line.startswith("# "):
                  return line.removeprefix("# ").strip()
          return fields["id"]
  word_count:
    returns: int
    source: |
      def compute(content, fields):
          return len(content.split())
`
	}
}
