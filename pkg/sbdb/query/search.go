package query

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
)

// SearchResult represents a full-text search match.
type SearchResult struct {
	ID      string
	File    string
	Snippet string
}

// Search performs a full-text grep search over markdown files.
// Falls back to a pure-Go scan if grep is not available.
func Search(s *schema.Schema, basePath, query string) ([]SearchResult, error) {
	docsDir := filepath.Join(basePath, s.DocsDir)

	// Try grep first
	results, err := grepSearch(docsDir, query)
	if err != nil {
		// Fallback to pure-Go search
		return pureGoSearch(docsDir, query)
	}
	return results, nil
}

func grepSearch(docsDir, query string) ([]SearchResult, error) {
	grepPath, err := exec.LookPath("grep")
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(grepPath, "-rl", "--include=*.md", query, docsDir)
	output, err := cmd.Output()
	if err != nil {
		// grep exits 1 if no matches found — that's not an error
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}

	var results []SearchResult
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		file := strings.TrimSpace(scanner.Text())
		if file == "" {
			continue
		}

		snippet, _ := extractSnippet(file, query)
		id := filenameToID(file)
		results = append(results, SearchResult{
			ID:      id,
			File:    file,
			Snippet: snippet,
		})
	}

	return results, nil
}

func pureGoSearch(docsDir, query string) ([]SearchResult, error) {
	var results []SearchResult
	lowerQuery := strings.ToLower(query)

	err := filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}

		content := string(data)
		if strings.Contains(strings.ToLower(content), lowerQuery) {
			snippet := extractSnippetFromContent(content, query)
			results = append(results, SearchResult{
				ID:      filenameToID(path),
				File:    path,
				Snippet: snippet,
			})
		}

		return nil
	})

	return results, err
}

func extractSnippet(file, query string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return extractSnippetFromContent(string(data), query), nil
}

func extractSnippetFromContent(content, query string) string {
	lower := strings.ToLower(content)
	lowerQ := strings.ToLower(query)
	idx := strings.Index(lower, lowerQ)
	if idx == -1 {
		return ""
	}

	start := idx - 40
	if start < 0 {
		start = 0
	}
	end := idx + len(query) + 40
	if end > len(content) {
		end = len(content)
	}

	snippet := content[start:end]
	snippet = strings.ReplaceAll(snippet, "\n", " ")
	return fmt.Sprintf("...%s...", strings.TrimSpace(snippet))
}

func filenameToID(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
