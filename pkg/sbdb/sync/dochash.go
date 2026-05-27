package sync

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ComputeDocHash returns a deterministic content hash for a markdown doc
// that is stable under whitespace-only changes.
//
// Process: split frontmatter from body; re-marshal the frontmatter via
// yaml.v3 in canonical form; trim trailing whitespace from the body; hash
// (canonical_yaml || "\n---\n" || trimmed_body).
//
// Returns a `sha256:<hex>` string for use in sidecars and CLI output.
func ComputeDocHash(doc []byte) (string, error) {
	fm, body, err := splitFrontmatter(doc)
	if err != nil {
		return "", err
	}
	var canonFM []byte
	if len(fm) > 0 {
		var node yaml.Node
		if err := yaml.Unmarshal(fm, &node); err != nil {
			return "", fmt.Errorf("frontmatter parse: %w", err)
		}
		canonFM, err = yaml.Marshal(&node)
		if err != nil {
			return "", fmt.Errorf("frontmatter remarshal: %w", err)
		}
	}
	body = bytes.TrimRight(body, " \t\r\n")
	h := sha256.New()
	h.Write(canonFM)
	h.Write([]byte("\n---\n"))
	h.Write(body)
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// splitFrontmatter is intentionally minimal: front-matter is a YAML block
// delimited by leading "---\n" and a closing "---\n". Anything else returns
// empty front-matter and the original bytes as body.
func splitFrontmatter(doc []byte) (fm, body []byte, err error) {
	if !bytes.HasPrefix(doc, []byte("---\n")) {
		return nil, doc, nil
	}
	rest := doc[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		// Tolerate EOF-terminated frontmatter.
		end = bytes.Index(rest, []byte("\n---"))
		if end < 0 {
			return nil, doc, fmt.Errorf("unterminated frontmatter")
		}
		return rest[:end], nil, nil
	}
	return rest[:end], rest[end+len("\n---\n"):], nil
}

// ParseFrontmatter returns the frontmatter map of a doc. Convenience for
// callers (resolver, validators) that need to walk frontmatter without
// re-implementing the split.
func ParseFrontmatter(doc []byte) (map[string]interface{}, error) {
	fm, _, err := splitFrontmatter(doc)
	if err != nil {
		return nil, err
	}
	if len(fm) == 0 {
		return map[string]interface{}{}, nil
	}
	var out map[string]interface{}
	if err := yaml.Unmarshal(fm, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]interface{}{}
	}
	return out, nil
}

// RenderedMarkdown returns the doc body (post-frontmatter) as a string.
// This is the value of the `rendered_markdown` payload source.
func RenderedMarkdown(doc []byte) (string, error) {
	_, body, err := splitFrontmatter(doc)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(body), " \t\r\n") + "\n", nil
}
