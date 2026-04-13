package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sergio-bershadsky/secondbrain-db/internal/output"
)

var initInteractive bool

func init() {
	initCmd.Flags().BoolVarP(&initInteractive, "interactive", "i", false, "run interactive setup wizard")
}

type wizardAnswers struct {
	ProjectName  string
	Entities     []string
	UseGitHub    bool
	UseVitePress bool
	UseIntegrity bool
	UseKG        bool
}

var entityDescriptions = map[string]string{
	"notes":      "Personal notes with tags, status tracking, and word count",
	"adr":        "Architecture Decision Records with status lifecycle and categories",
	"discussion": "Meeting notes and discussions with participants and monthly sharding",
	"task":       "Task tracking with priority, assignee, checklists, and due dates",
	"blog":       "Blog posts with publish/draft status and reading time",
}

func runInteractiveInit(basePath, format string) error {
	scanner := bufio.NewScanner(os.Stdin)
	answers := wizardAnswers{}

	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║     secondbrain-db — Project Setup Wizard    ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	// 1. Project name
	answers.ProjectName = ask(scanner, "Project name", filepath.Base(basePath))

	// 2. Entities
	fmt.Println("\n── Which entities do you need? ──")
	fmt.Println()
	for _, name := range []string{"notes", "adr", "discussion", "task", "blog"} {
		fmt.Printf("  [%s] %s\n", name, entityDescriptions[name])
	}
	fmt.Println()
	selected := ask(scanner, "Select entities (comma-separated)", "notes,adr,discussion")
	for _, e := range strings.Split(selected, ",") {
		e = strings.TrimSpace(e)
		if e != "" {
			answers.Entities = append(answers.Entities, e)
		}
	}
	if len(answers.Entities) == 0 {
		answers.Entities = []string{"notes"}
	}

	// 3. GitHub hosting
	fmt.Println()
	answers.UseGitHub = askYesNo(scanner, "Hosting on GitHub? (adds CI workflow with doctor checks)", true)

	// 4. VitePress
	answers.UseVitePress = askYesNo(scanner, "Using VitePress for documentation site?", false)

	// 5. Integrity
	answers.UseIntegrity = askYesNo(scanner, "Enable integrity signing? (SHA-256 + HMAC tamper detection)", true)

	// 6. Knowledge graph
	answers.UseKG = askYesNo(scanner, "Enable knowledge graph? (link extraction + semantic search)", true)

	// Confirm
	fmt.Println("\n── Summary ──")
	fmt.Printf("  Project:    %s\n", answers.ProjectName)
	fmt.Printf("  Entities:   %s\n", strings.Join(answers.Entities, ", "))
	fmt.Printf("  GitHub CI:  %v\n", answers.UseGitHub)
	fmt.Printf("  VitePress:  %v\n", answers.UseVitePress)
	fmt.Printf("  Integrity:  %v\n", answers.UseIntegrity)
	fmt.Printf("  KG:         %v\n", answers.UseKG)
	fmt.Println()

	if !askYesNo(scanner, "Proceed?", true) {
		fmt.Println("Aborted.")
		return nil
	}

	// Generate project
	return generateProject(basePath, format, answers)
}

func generateProject(basePath, format string, answers wizardAnswers) error {
	fmt.Println()

	// Create directories
	dirs := []string{"schemas", "docs", "data"}
	for _, entity := range answers.Entities {
		tpl := templateSchema(entity)
		if tpl == "" {
			continue
		}
		// Parse to get entity name for dirs
		entityName := entity
		switch entity {
		case "adr":
			entityName = "decisions"
			dirs = append(dirs, "docs/decisions", "data/decisions")
		case "discussion":
			entityName = "discussions"
			dirs = append(dirs, "docs/discussions", "data/discussions")
		case "task":
			entityName = "tasks"
			dirs = append(dirs, "docs/tasks", "data/tasks")
		case "blog":
			entityName = "posts"
			dirs = append(dirs, "docs/posts", "data/posts")
		default:
			dirs = append(dirs, "docs/"+entityName, "data/"+entityName)
		}
		_ = entityName
	}

	for _, d := range dirs {
		os.MkdirAll(filepath.Join(basePath, d), 0o755)
	}
	fmt.Println("  ✓ Created directories")

	// Write schemas
	for _, entity := range answers.Entities {
		content := templateSchema(entity)
		if content == "" {
			continue
		}
		path := filepath.Join(basePath, "schemas", entity+".yaml")
		os.WriteFile(path, []byte(content), 0o644)
		fmt.Printf("  ✓ Created schemas/%s.yaml\n", entity)
	}

	// Write .sbdb.toml
	defaultSchema := answers.Entities[0]
	integrityMode := "strict"
	if !answers.UseIntegrity {
		integrityMode = "off"
	}

	toml := fmt.Sprintf(`schema_dir = "./schemas"
base_path = "."
default_schema = %q

[output]
format = "auto"

[integrity]
key_source = "env"
`, defaultSchema)

	if answers.UseKG {
		toml += `
[knowledge_graph]
enabled = true
db_path = "data/.sbdb.db"

[knowledge_graph.embeddings]
provider = "openai"
model = "text-embedding-3-small"
dimension = 1536

[knowledge_graph.graph]
auto_index = true
extract_links = true
`
	}

	_ = integrityMode
	os.WriteFile(filepath.Join(basePath, ".sbdb.toml"), []byte(toml), 0o644)
	fmt.Println("  ✓ Created .sbdb.toml")

	// Write .gitignore
	gitignore := `node_modules/
.vitepress/cache/
.vitepress/dist/
data/.sbdb.db
data/.sbdb-graph.db
.sbdb-integrity.key
*.tmp
`
	os.WriteFile(filepath.Join(basePath, ".gitignore"), []byte(gitignore), 0o644)
	fmt.Println("  ✓ Created .gitignore")

	// GitHub Actions
	if answers.UseGitHub {
		ghDir := filepath.Join(basePath, ".github", "workflows")
		os.MkdirAll(ghDir, 0o755)

		doctorYaml := `name: KB Health Check

on:
  push:
    branches: [main]
    paths:
      - "docs/**"
      - "data/**"
      - "schemas/**"
  pull_request:
    paths:
      - "docs/**"
      - "data/**"
      - "schemas/**"

permissions:
  contents: read

jobs:
  doctor:
    name: Doctor Check
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.23"
          cache: true
      - uses: sergio-bershadsky/secondbrain-db/.github/actions/doctor@main
`
		os.WriteFile(filepath.Join(ghDir, "doctor.yml"), []byte(doctorYaml), 0o644)
		fmt.Println("  ✓ Created .github/workflows/doctor.yml")
	}

	// VitePress scaffold
	if answers.UseVitePress {
		vpDir := filepath.Join(basePath, "docs", ".vitepress")
		os.MkdirAll(vpDir, 0o755)

		configTS := fmt.Sprintf(`import { defineConfig } from 'vitepress'

export default defineConfig({
  title: '%s',
  description: 'Knowledge Base',
  themeConfig: {
    nav: [
      { text: 'Home', link: '/' },
    ],
    sidebar: {
`, answers.ProjectName)

		for _, entity := range answers.Entities {
			section := entity
			switch entity {
			case "adr":
				section = "decisions"
			case "discussion":
				section = "discussions"
			case "task":
				section = "tasks"
			case "blog":
				section = "posts"
			}
			configTS += fmt.Sprintf("      '/%s/': [{ text: '%s', link: '/%s/' }],\n",
				section, capitalize(section), section)
		}

		configTS += `    }
  }
})
`
		os.WriteFile(filepath.Join(vpDir, "config.ts"), []byte(configTS), 0o644)

		// Package.json
		pkg := fmt.Sprintf(`{
  "name": "%s",
  "private": true,
  "scripts": {
    "dev": "vitepress dev docs",
    "build": "vitepress build docs",
    "preview": "vitepress preview docs"
  },
  "devDependencies": {
    "vitepress": "^1.5.0"
  }
}
`, answers.ProjectName)
		os.WriteFile(filepath.Join(basePath, "package.json"), []byte(pkg), 0o644)

		// Landing page
		index := fmt.Sprintf(`---
layout: home
hero:
  name: "%s"
  tagline: "Knowledge Base"
  actions:
    - theme: brand
      text: Get Started
      link: /%s/
---
`, answers.ProjectName, func() string {
			switch answers.Entities[0] {
			case "adr":
				return "decisions"
			case "discussion":
				return "discussions"
			case "task":
				return "tasks"
			case "blog":
				return "posts"
			default:
				return answers.Entities[0]
			}
		}())
		os.WriteFile(filepath.Join(basePath, "docs", "index.md"), []byte(index), 0o644)

		fmt.Println("  ✓ Created VitePress scaffold")
	}

	fmt.Println()
	fmt.Println("Done! Next steps:")
	fmt.Printf("  cd %s\n", basePath)
	fmt.Println("  sbdb schema list")
	if answers.UseVitePress {
		fmt.Println("  npm install && npm run dev")
	}
	if answers.UseIntegrity {
		fmt.Println("  sbdb doctor init-key")
	}

	return output.PrintData(format, map[string]any{
		"action":    "init",
		"project":   answers.ProjectName,
		"entities":  answers.Entities,
		"github_ci": answers.UseGitHub,
		"vitepress": answers.UseVitePress,
		"integrity": answers.UseIntegrity,
		"kg":        answers.UseKG,
	})
}

func ask(scanner *bufio.Scanner, prompt, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("  %s [%s]: ", prompt, defaultVal)
	} else {
		fmt.Printf("  %s: ", prompt)
	}

	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input != "" {
			return input
		}
	}
	return defaultVal
}

func askYesNo(scanner *bufio.Scanner, prompt string, defaultYes bool) bool {
	def := "Y/n"
	if !defaultYes {
		def = "y/N"
	}
	fmt.Printf("  %s [%s]: ", prompt, def)

	if scanner.Scan() {
		input := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if input == "" {
			return defaultYes
		}
		return input == "y" || input == "yes"
	}
	return defaultYes
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
