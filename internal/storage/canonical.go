package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// CanonicalHash returns the SHA-256 hex digest of the canonical representation of a value.
func CanonicalHash(v any) string {
	h := sha256.New()
	h.Write([]byte(CanonicalString(v)))
	return hex.EncodeToString(h.Sum(nil))
}

// CanonicalString produces a stable, deterministic string representation of a value.
// Maps are sorted by key recursively. This is used for integrity hashing —
// YAML reformatting (key order, quoting, flow style) must not change the output.
func CanonicalString(v any) string {
	var b strings.Builder
	writeCanonical(&b, v, 0)
	return b.String()
}

func writeCanonical(b *strings.Builder, v any, depth int) {
	switch val := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if val {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case int:
		fmt.Fprintf(b, "%d", val)
	case int64:
		fmt.Fprintf(b, "%d", val)
	case float64:
		// Normalize: integers as int, others as float
		if val == float64(int64(val)) {
			fmt.Fprintf(b, "%d", int64(val))
		} else {
			fmt.Fprintf(b, "%g", val)
		}
	case string:
		// Quote strings to distinguish "null" from null, "123" from 123
		fmt.Fprintf(b, "%q", val)
	case map[string]any:
		writeCanonicalMap(b, val, depth)
	case []any:
		writeCanonicalSlice(b, val, depth)
	default:
		// Fallback: use %v for unknown types (dates parsed as strings by YAML)
		fmt.Fprintf(b, "%q", fmt.Sprintf("%v", val))
	}
}

func writeCanonicalMap(b *strings.Builder, m map[string]any, depth int) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	b.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(b, "%q:", k)
		writeCanonical(b, m[k], depth+1)
	}
	b.WriteString("}")
}

func writeCanonicalSlice(b *strings.Builder, s []any, depth int) {
	b.WriteString("[")
	for i, item := range s {
		if i > 0 {
			b.WriteString(",")
		}
		writeCanonical(b, item, depth+1)
	}
	b.WriteString("]")
}

// CanonicalBodyHash returns the SHA-256 hex digest of a markdown body.
// Normalizes trailing newline: always exactly one.
func CanonicalBodyHash(body string) string {
	normalized := strings.TrimRight(body, "\n") + "\n"
	h := sha256.New()
	h.Write([]byte(normalized))
	return hex.EncodeToString(h.Sum(nil))
}
