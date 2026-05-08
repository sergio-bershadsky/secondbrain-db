package schema

import (
	"fmt"
	"net/url"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseJSONSchema parses a JSON Schema 2020-12 + x-* document into the
// internal Schema struct.
func ParseJSONSchema(data []byte) (*Schema, error) {
	var top map[string]any
	if err := yaml.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("schema: parse: %w", err)
	}

	s := &Schema{
		Version:    1,
		Fields:     FieldMap{},
		Virtuals:   map[string]*Virtual{},
		EventTypes: map[string]*EventType{},
	}

	if v, ok := top["x-schema-version"]; ok {
		switch t := v.(type) {
		case int:
			s.Version = t
		case string:
			head, _, _ := strings.Cut(t, ".")
			fmt.Sscanf(head, "%d", &s.Version)
		}
	}
	if v, ok := top["x-entity"].(string); ok {
		s.Entity = v
	}
	if v, ok := top["x-id"].(string); ok {
		s.IDField = v
	}
	if v, ok := top["x-integrity"].(string); ok {
		s.Integrity = v
	}
	if storage, ok := top["x-storage"].(map[string]any); ok {
		if d, ok := storage["docs_dir"].(string); ok {
			s.DocsDir = d
		}
		if f, ok := storage["filename"].(string); ok {
			s.Filename = f
		}
		if r, ok := storage["records_dir"].(string); ok {
			s.RecordsDir = r
		}
	}
	if part, ok := top["x-partition"].(map[string]any); ok {
		if m, ok := part["mode"].(string); ok {
			s.Partition = m
		}
		if f, ok := part["field"].(string); ok {
			s.DateField = f
		}
	}
	if events, ok := top["x-events"].(map[string]any); ok {
		if b, ok := events["bucket"].(string); ok {
			s.Bucket = b
		}
		_ = events["types"]
	}

	requiredSet := map[string]bool{}
	if req, ok := top["required"].([]any); ok {
		for _, r := range req {
			if name, ok := r.(string); ok {
				requiredSet[name] = true
			}
		}
	}

	props, _ := top["properties"].(map[string]any)
	for name, raw := range props {
		propMap, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("schema: properties.%s: expected object", name)
		}
		readOnly, _ := propMap["readOnly"].(bool)
		comp, _ := propMap["x-compute"].(map[string]any)
		if readOnly && comp != nil {
			v := &Virtual{Name: name}
			if src, ok := comp["source"].(string); ok {
				v.Source = src
			}
			if e, ok := comp["edge"].(bool); ok {
				v.Edge = e
			}
			if ent, ok := comp["edge_entity"].(string); ok {
				v.EdgeEntity = ent
			}
			v.Returns = jsonTypeToReturns(propMap)
			s.Virtuals[name] = v
			continue
		}
		field, err := propToField(name, propMap)
		if err != nil {
			return nil, err
		}
		field.Required = requiredSet[name]
		s.Fields[name] = field
	}

	if err := s.Validate(); err != nil {
		return nil, err
	}
	return s, nil
}

func propToField(name string, m map[string]any) (*Field, error) {
	f := &Field{Name: name}
	if ref, ok := m["$ref"].(string); ok {
		ent, err := refEntityFromURI(ref)
		if err != nil {
			return nil, fmt.Errorf("properties.%s: %w", name, err)
		}
		f.Type = FieldTypeRef
		f.RefEntity = ent
		return f, nil
	}
	if enumRaw, ok := m["enum"].([]any); ok {
		f.Type = FieldTypeEnum
		for _, e := range enumRaw {
			if s, ok := e.(string); ok {
				f.Values = append(f.Values, s)
			}
		}
		if d, ok := m["default"]; ok {
			f.Default = d
		}
		return f, nil
	}
	t, _ := m["type"].(string)
	switch t {
	case "string":
		if format, ok := m["format"].(string); ok {
			switch format {
			case "date":
				f.Type = FieldTypeDate
			case "date-time":
				f.Type = FieldTypeDatetime
			default:
				f.Type = FieldTypeString
			}
		} else {
			f.Type = FieldTypeString
		}
	case "integer":
		f.Type = FieldTypeInt
	case "number":
		f.Type = FieldTypeFloat
	case "boolean":
		f.Type = FieldTypeBool
	case "array":
		f.Type = FieldTypeList
		if items, ok := m["items"].(map[string]any); ok {
			it, err := propToField("items", items)
			if err != nil {
				return nil, err
			}
			f.Items = it
		}
	case "object":
		f.Type = FieldTypeObject
		f.Fields = FieldMap{}
		nestedReq := map[string]bool{}
		if req, ok := m["required"].([]any); ok {
			for _, r := range req {
				if rs, ok := r.(string); ok {
					nestedReq[rs] = true
				}
			}
		}
		if nested, ok := m["properties"].(map[string]any); ok {
			for nname, nraw := range nested {
				nmap, ok := nraw.(map[string]any)
				if !ok {
					continue
				}
				nf, err := propToField(nname, nmap)
				if err != nil {
					return nil, err
				}
				nf.Required = nestedReq[nname]
				f.Fields[nname] = nf
			}
		}
	default:
		f.Type = FieldTypeString
	}
	if d, ok := m["default"]; ok {
		f.Default = d
	}
	return f, nil
}

func refEntityFromURI(ref string) (string, error) {
	u, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("invalid $ref %q: %w", ref, err)
	}
	if u.Scheme != "sbdb" {
		return "", fmt.Errorf("$ref must use sbdb:// scheme, got %q", ref)
	}
	if u.Host == "" {
		return "", fmt.Errorf("$ref missing entity name: %q", ref)
	}
	return u.Host, nil
}

func jsonTypeToReturns(m map[string]any) string {
	if t, ok := m["type"].(string); ok {
		switch t {
		case "string":
			if f, _ := m["format"].(string); f == "date" {
				return "date"
			} else if f == "date-time" {
				return "datetime"
			}
			return "string"
		case "integer":
			return "int"
		case "number":
			return "float"
		case "boolean":
			return "bool"
		case "array":
			if items, ok := m["items"].(map[string]any); ok {
				return "list[" + jsonTypeToReturns(items) + "]"
			}
			return "list"
		}
	}
	return "any"
}
