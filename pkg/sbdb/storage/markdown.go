package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseMarkdown reads a markdown file and splits it into frontmatter and body.
// Frontmatter is the YAML between the first pair of --- delimiters.
// Returns empty frontmatter if no valid frontmatter block is found.
func ParseMarkdown(path string) (frontmatter map[string]any, body string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("reading markdown file: %w", err)
	}
	return ParseMarkdownBytes(data)
}

// ParseMarkdownBytes parses frontmatter and body from raw bytes.
func ParseMarkdownBytes(data []byte) (frontmatter map[string]any, body string, err error) {
	content := string(data)

	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return map[string]any{}, content, nil
	}

	// Find the closing ---
	rest := content[4:] // skip opening "---\n"
	idx := strings.Index(rest, "\n---\n")
	if idx == -1 {
		// Try with \r\n
		idx = strings.Index(rest, "\r\n---\r\n")
		if idx == -1 {
			// Check if --- is at the very end
			if strings.HasSuffix(strings.TrimRight(rest, "\r\n"), "\n---") || strings.HasSuffix(strings.TrimRight(rest, "\r\n"), "\r\n---") {
				idx = strings.LastIndex(rest, "---") - 1
			}
			if idx == -1 {
				return map[string]any{}, content, nil
			}
		}
	}

	yamlContent := rest[:idx]
	body = strings.TrimLeft(rest[idx+5:], "\n\r") // skip "\n---\n"

	frontmatter = make(map[string]any)
	if err := yaml.Unmarshal([]byte(yamlContent), &frontmatter); err != nil {
		return nil, "", fmt.Errorf("parsing frontmatter YAML: %w", err)
	}

	if frontmatter == nil {
		frontmatter = make(map[string]any)
	}

	return frontmatter, body, nil
}

// WriteMarkdown writes a markdown file with frontmatter and body atomically.
// Uses a temp file + rename to prevent partial writes.
func WriteMarkdown(path string, frontmatter map[string]any, body string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	content, err := RenderMarkdown(frontmatter, body)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".sbdb-*.md.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}

// RenderMarkdown produces the string content of a markdown file with frontmatter.
func RenderMarkdown(frontmatter map[string]any, body string) (string, error) {
	var b strings.Builder

	if len(frontmatter) > 0 {
		yamlBytes, err := yaml.Marshal(frontmatter)
		if err != nil {
			return "", fmt.Errorf("marshaling frontmatter: %w", err)
		}
		b.WriteString("---\n")
		b.Write(yamlBytes)
		b.WriteString("---\n")
	}

	if body != "" {
		if len(frontmatter) > 0 {
			b.WriteString("\n")
		}
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteString("\n")
		}
	}

	return b.String(), nil
}
