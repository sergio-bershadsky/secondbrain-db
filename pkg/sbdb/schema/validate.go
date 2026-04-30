package schema

import (
	"fmt"
	"strings"
	"time"
)

// ValidationError represents a single validation failure with a field path.
type ValidationError struct {
	Path    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors []*ValidationError

func (ve ValidationErrors) Error() string {
	var msgs []string
	for _, e := range ve {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}

// ValidateRecord checks a data map against a schema and returns all validation errors.
func ValidateRecord(s *Schema, data map[string]any) ValidationErrors {
	var errs ValidationErrors
	for name, field := range s.Fields {
		val, exists := data[name]
		if !exists || val == nil {
			if field.Required {
				errs = append(errs, &ValidationError{Path: name, Message: "missing required field"})
			}
			continue
		}
		errs = append(errs, validateValue(name, field, val)...)
	}
	return errs
}

func validateValue(path string, field *Field, val any) ValidationErrors {
	var errs ValidationErrors

	switch field.Type {
	case FieldTypeString:
		if _, ok := asString(val); !ok {
			errs = append(errs, &ValidationError{Path: path, Message: fmt.Sprintf("expected string, got %T", val)})
		}

	case FieldTypeInt:
		if _, ok := asInt(val); !ok {
			errs = append(errs, &ValidationError{Path: path, Message: fmt.Sprintf("expected int, got %T", val)})
		}

	case FieldTypeFloat:
		if _, ok := asFloat(val); !ok {
			errs = append(errs, &ValidationError{Path: path, Message: fmt.Sprintf("expected float, got %T", val)})
		}

	case FieldTypeBool:
		if _, ok := val.(bool); !ok {
			errs = append(errs, &ValidationError{Path: path, Message: fmt.Sprintf("expected bool, got %T", val)})
		}

	case FieldTypeDate, FieldTypeDatetime:
		if s, ok := asString(val); ok {
			if _, err := parseAnyDate(s); err != nil {
				errs = append(errs, &ValidationError{Path: path, Message: fmt.Sprintf("invalid date: %s", s)})
			}
		} else if _, ok := val.(time.Time); !ok {
			errs = append(errs, &ValidationError{Path: path, Message: fmt.Sprintf("expected date string, got %T", val)})
		}

	case FieldTypeEnum:
		s, ok := asString(val)
		if !ok {
			errs = append(errs, &ValidationError{Path: path, Message: fmt.Sprintf("expected string for enum, got %T", val)})
		} else {
			found := false
			for _, v := range field.Values {
				if v == s {
					found = true
					break
				}
			}
			if !found {
				errs = append(errs, &ValidationError{Path: path, Message: fmt.Sprintf("value %q not in enum %v", s, field.Values)})
			}
		}

	case FieldTypeList:
		slice, ok := asSlice(val)
		if !ok {
			errs = append(errs, &ValidationError{Path: path, Message: fmt.Sprintf("expected list, got %T", val)})
		} else if field.Items != nil {
			for i, item := range slice {
				itemPath := fmt.Sprintf("%s[%d]", path, i)
				errs = append(errs, validateValue(itemPath, field.Items, item)...)
			}
		}

	case FieldTypeObject:
		m, ok := asMap(val)
		if !ok {
			errs = append(errs, &ValidationError{Path: path, Message: fmt.Sprintf("expected object, got %T", val)})
		} else {
			for subName, subField := range field.Fields {
				subVal, exists := m[subName]
				subPath := path + "." + subName
				if !exists || subVal == nil {
					if subField.Required {
						errs = append(errs, &ValidationError{Path: subPath, Message: "missing required field"})
					}
					continue
				}
				errs = append(errs, validateValue(subPath, subField, subVal)...)
			}
		}
	}

	return errs
}

func asString(v any) (string, bool) {
	switch val := v.(type) {
	case string:
		return val, true
	case fmt.Stringer:
		return val.String(), true
	default:
		return "", false
	}
}

func asInt(v any) (int64, bool) {
	switch val := v.(type) {
	case int:
		return int64(val), true
	case int64:
		return val, true
	case float64:
		if val == float64(int64(val)) {
			return int64(val), true
		}
	}
	return 0, false
}

func asFloat(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	}
	return 0, false
}

func asSlice(v any) ([]any, bool) {
	switch val := v.(type) {
	case []any:
		return val, true
	case []string:
		result := make([]any, len(val))
		for i, s := range val {
			result[i] = s
		}
		return result, true
	default:
		return nil, false
	}
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func parseAnyDate(s string) (time.Time, error) {
	for _, layout := range []string{
		"2006-01-02",
		"2006-01-02T15:04:05Z07:00",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse date %q", s)
}
