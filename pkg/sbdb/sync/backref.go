package sync

import (
	"fmt"
	"strings"
)

// ResolveBackref walks a dotted path inside a parsed frontmatter map and
// returns the leaf as a string. Numeric leaves are coerced via fmt.Sprint.
// Returns ok=false when the path is missing or terminates at a non-leaf.
func ResolveBackref(frontmatter map[string]interface{}, path string) (string, bool) {
	if path == "" {
		return "", false
	}
	parts := strings.Split(path, ".")
	var cur interface{} = frontmatter
	for _, p := range parts {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return "", false
		}
		v, exists := m[p]
		if !exists {
			return "", false
		}
		cur = v
	}
	switch v := cur.(type) {
	case string:
		if v == "" {
			return "", false
		}
		return v, true
	case int, int32, int64, float32, float64:
		return fmt.Sprint(v), true
	}
	return "", false
}
