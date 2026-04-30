package schema

// ClassifyFields splits a schema's fields into scalar (records+frontmatter) and complex (frontmatter-only).
func ClassifyFields(s *Schema) (scalar []string, complex_ []string) {
	for name, f := range s.Fields {
		if f.Type.IsScalar() {
			scalar = append(scalar, name)
		} else {
			complex_ = append(complex_, name)
		}
	}
	return scalar, complex_
}

// BuildRecordData extracts scalar fields + scalar virtuals from a full data map.
// This is the projection stored in records.yaml.
func BuildRecordData(s *Schema, fullData map[string]any, virtualData map[string]any) map[string]any {
	record := make(map[string]any)

	// Scalar schema fields
	for name, f := range s.Fields {
		if f.Type.IsScalar() {
			if val, ok := fullData[name]; ok {
				record[name] = val
			}
		}
	}

	// Scalar virtual fields
	for name, v := range s.Virtuals {
		if v.IsScalarReturn() {
			if val, ok := virtualData[name]; ok {
				record[name] = val
			}
		}
	}

	return record
}

// BuildFrontmatterData combines all fields + all virtuals for frontmatter storage.
func BuildFrontmatterData(s *Schema, fullData map[string]any, virtualData map[string]any) map[string]any {
	fm := make(map[string]any)

	// All schema fields
	for name := range s.Fields {
		if val, ok := fullData[name]; ok {
			fm[name] = val
		}
	}

	// All virtuals (scalar + complex)
	for name := range s.Virtuals {
		if val, ok := virtualData[name]; ok {
			fm[name] = val
		}
	}

	return fm
}
