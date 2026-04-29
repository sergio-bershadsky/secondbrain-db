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
	UseGitHub    bool
	UseVitePress bool
	UseIntegrity bool
	UseKG        bool
}

func runInteractiveInit(basePath, format string) error {
	scanner := bufio.NewScanner(os.Stdin)
	answers := wizardAnswers{}

	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║     secondbrain-db — Project Setup Wizard    ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	answers.ProjectName = ask(scanner, "Project name", filepath.Base(basePath))

	fmt.Println()
	answers.UseGitHub = askYesNo(scanner, "Hosting on GitHub? (adds CI workflow with doctor checks)", true)
	answers.UseVitePress = askYesNo(scanner, "Using VitePress for documentation site?", false)
	answers.UseIntegrity = askYesNo(scanner, "Enable integrity signing? (SHA-256 + HMAC tamper detection)", true)
	answers.UseKG = askYesNo(scanner, "Enable knowledge graph? (link extraction + semantic search)", true)

	fmt.Println("\n── Summary ──")
	fmt.Printf("  Project:    %s\n", answers.ProjectName)
	fmt.Printf("  GitHub CI:  %v\n", answers.UseGitHub)
	fmt.Printf("  VitePress:  %v\n", answers.UseVitePress)
	fmt.Printf("  Integrity:  %v\n", answers.UseIntegrity)
	fmt.Printf("  KG:         %v\n", answers.UseKG)
	fmt.Println()

	if !askYesNo(scanner, "Proceed?", true) {
		fmt.Println("Aborted.")
		return nil
	}

	return generateProject(basePath, format, answers)
}

func generateProject(basePath, format string, answers wizardAnswers) error {
	fmt.Println()
	for _, d := range []string{"schemas", "docs"} {
		os.MkdirAll(filepath.Join(basePath, d), 0o755)
	}
	fmt.Println("  ✓ Created directories")

	toml := `schema_dir = "./schemas"
base_path = "."

[output]
format = "auto"

[integrity]
key_source = "env"
`

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

	os.WriteFile(filepath.Join(basePath, ".sbdb.toml"), []byte(toml), 0o644)
	fmt.Println("  ✓ Created .sbdb.toml")

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

	if answers.UseGitHub {
		ghDir := filepath.Join(basePath, ".github", "workflows")
		os.MkdirAll(ghDir, 0o755)
		doctorYaml := `name: KB Health Check

on:
  push:
    branches: [main]
    paths:
      - "docs/**"
      - "schemas/**"
  pull_request:
    paths:
      - "docs/**"
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
      // Wire up sections after you add schemas under schemas/<entity>.yaml.
    }
  }
})
`, answers.ProjectName)
		os.WriteFile(filepath.Join(vpDir, "config.ts"), []byte(configTS), 0o644)

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

		index := fmt.Sprintf(`---
layout: home
hero:
  name: "%s"
  tagline: "Knowledge Base"
---
`, answers.ProjectName)
		os.WriteFile(filepath.Join(basePath, "docs", "index.md"), []byte(index), 0o644)
		fmt.Println("  ✓ Created VitePress scaffold")
	}

	fmt.Println()
	fmt.Println("Done! Next steps:")
	fmt.Printf("  cd %s\n", basePath)
	fmt.Println("  # Add a schema under schemas/<entity>.yaml — copy a reference")
	fmt.Println("  # from the secondbrain-db Claude Code plugin to start, or write your own.")
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
