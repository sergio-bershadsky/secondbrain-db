package query

import (
	"fmt"
	"strings"
)

// Lookup represents a filter condition parsed from a key like "status__gte".
type Lookup struct {
	Field    string
	Operator string // "", "gte", "lte", "gt", "lt", "in", "contains", "icontains", "startswith"
	Value    any
}

// ParseLookup splits a filter key into field name and operator.
func ParseLookup(key string, value any) Lookup {
	operators := []string{
		"__icontains", "__contains", "__startswith",
		"__gte", "__lte", "__gt", "__lt", "__in",
	}
	for _, op := range operators {
		if strings.HasSuffix(key, op) {
			return Lookup{
				Field:    strings.TrimSuffix(key, op),
				Operator: strings.TrimPrefix(op, "__"),
				Value:    value,
			}
		}
	}
	return Lookup{Field: key, Operator: "", Value: value}
}

// Match checks if a record field value satisfies a lookup condition.
func (l Lookup) Match(record map[string]any) bool {
	fieldVal, exists := record[l.Field]
	if !exists {
		return false
	}

	switch l.Operator {
	case "": // exact match
		return fmt.Sprintf("%v", fieldVal) == fmt.Sprintf("%v", l.Value)

	case "gte":
		return compareValues(fieldVal, l.Value) >= 0

	case "lte":
		return compareValues(fieldVal, l.Value) <= 0

	case "gt":
		return compareValues(fieldVal, l.Value) > 0

	case "lt":
		return compareValues(fieldVal, l.Value) < 0

	case "contains":
		return stringContains(fieldVal, l.Value, false)

	case "icontains":
		return stringContains(fieldVal, l.Value, true)

	case "startswith":
		s := fmt.Sprintf("%v", fieldVal)
		prefix := fmt.Sprintf("%v", l.Value)
		return strings.HasPrefix(s, prefix)

	case "in":
		return valueIn(fieldVal, l.Value)

	default:
		return false
	}
}

func compareValues(a, b any) int {
	sa := fmt.Sprintf("%v", a)
	sb := fmt.Sprintf("%v", b)

	// Try numeric comparison
	if fa, fb, ok := asFloats(a, b); ok {
		if fa < fb {
			return -1
		}
		if fa > fb {
			return 1
		}
		return 0
	}

	// Fall back to string comparison
	if sa < sb {
		return -1
	}
	if sa > sb {
		return 1
	}
	return 0
}

func asFloats(a, b any) (float64, float64, bool) {
	fa, oka := toFloat(a)
	fb, okb := toFloat(b)
	return fa, fb, oka && okb
}

func toFloat(v any) (float64, bool) {
	switch val := v.(type) {
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case float64:
		return val, true
	default:
		return 0, false
	}
}

func stringContains(fieldVal, searchVal any, ignoreCase bool) bool {
	s := fmt.Sprintf("%v", fieldVal)
	sub := fmt.Sprintf("%v", searchVal)
	if ignoreCase {
		s = strings.ToLower(s)
		sub = strings.ToLower(sub)
	}
	return strings.Contains(s, sub)
}

func valueIn(fieldVal, listVal any) bool {
	fieldStr := fmt.Sprintf("%v", fieldVal)

	switch vals := listVal.(type) {
	case []any:
		for _, v := range vals {
			if fmt.Sprintf("%v", v) == fieldStr {
				return true
			}
		}
	case []string:
		for _, v := range vals {
			if v == fieldStr {
				return true
			}
		}
	case string:
		// Support comma-separated: "a,b,c"
		for _, v := range strings.Split(vals, ",") {
			if strings.TrimSpace(v) == fieldStr {
				return true
			}
		}
	}
	return false
}
