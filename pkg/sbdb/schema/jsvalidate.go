package schema

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// Validator wraps a compiled JSON Schema for an sbdb entity.
type Validator struct {
	compiled *jsonschema.Schema
}

// NewValidator compiles a Schema's data shape into a runtime JSON Schema validator.
//
// Virtuals are excluded — they are computed, not user-supplied. The data
// passed to ValidateMap is the user-supplied frontmatter only.
func NewValidator(s *Schema) (*Validator, error) {
	doc, err := schemaToJSONSchemaDoc(s)
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	url := "inmem://" + s.Entity
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("schema: register: %w", err)
	}
	cs, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("schema: compile: %w", err)
	}
	return &Validator{compiled: cs}, nil
}

// ValidateMap validates a record's data map against the compiled schema.
func (v *Validator) ValidateMap(m map[string]any) error {
	// Round-trip via JSON so types match the validator's expectations
	// (yaml.v3 may produce map[interface{}]interface{} or different numeric types).
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	var anyVal any
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&anyVal); err != nil {
		return err
	}
	return v.compiled.Validate(anyVal)
}

// schemaToJSONSchemaDoc emits a generic map suitable for jsonschema.AddResource.
// It strips x-* keys (they don't affect data validation) and excludes virtuals.
func schemaToJSONSchemaDoc(s *Schema) (any, error) {
	doc := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "object",
	}
	props := map[string]any{}
	var required []string
	for name, f := range s.Fields {
		props[name] = fieldToJSONSchema(f)
		if f.Required {
			required = append(required, name)
		}
	}
	doc["properties"] = props
	if len(required) > 0 {
		doc["required"] = required
	}
	// Round-trip via YAML to normalise nested types into json-friendly shapes.
	bs, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var out any
	if err := yaml.Unmarshal(bs, &out); err != nil {
		return nil, err
	}
	return normaliseYAMLForJSON(out), nil
}

func fieldToJSONSchema(f *Field) any {
	m := map[string]any{}
	switch f.Type {
	case FieldTypeString:
		m["type"] = "string"
	case FieldTypeInt:
		m["type"] = "integer"
	case FieldTypeFloat:
		m["type"] = "number"
	case FieldTypeBool:
		m["type"] = "boolean"
	case FieldTypeDate:
		m["type"] = "string"
		m["format"] = "date"
	case FieldTypeDatetime:
		m["type"] = "string"
		m["format"] = "date-time"
	case FieldTypeEnum:
		m["enum"] = stringsToAny(f.Values)
	case FieldTypeList:
		m["type"] = "array"
		if f.Items != nil {
			m["items"] = fieldToJSONSchema(f.Items)
		}
	case FieldTypeObject:
		m["type"] = "object"
		nested := map[string]any{}
		var req []string
		for nname, nf := range f.Fields {
			nested[nname] = fieldToJSONSchema(nf)
			if nf.Required {
				req = append(req, nname)
			}
		}
		m["properties"] = nested
		if len(req) > 0 {
			m["required"] = req
		}
	case FieldTypeRef:
		// Refs are strings at the data layer; existence is checked separately.
		m["type"] = "string"
	}
	return m
}

func stringsToAny(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

// normaliseYAMLForJSON converts map[interface{}]interface{} (which yaml.v3
// produces in some cases) to map[string]any so the validator can reason
// about it. Recurses through nested structures.
func normaliseYAMLForJSON(v any) any {
	switch t := v.(type) {
	case map[any]any:
		m := map[string]any{}
		for k, vv := range t {
			m[fmt.Sprint(k)] = normaliseYAMLForJSON(vv)
		}
		return m
	case map[string]any:
		for k, vv := range t {
			t[k] = normaliseYAMLForJSON(vv)
		}
		return t
	case []any:
		for i, vv := range t {
			t[i] = normaliseYAMLForJSON(vv)
		}
		return t
	default:
		return v
	}
}
