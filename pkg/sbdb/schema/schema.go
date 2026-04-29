package schema

import "fmt"

// FieldType represents the type of a schema field.
type FieldType string

const (
	FieldTypeString   FieldType = "string"
	FieldTypeInt      FieldType = "int"
	FieldTypeFloat    FieldType = "float"
	FieldTypeBool     FieldType = "bool"
	FieldTypeDate     FieldType = "date"
	FieldTypeDatetime FieldType = "datetime"
	FieldTypeEnum     FieldType = "enum"
	FieldTypeList     FieldType = "list"
	FieldTypeObject   FieldType = "object"
	FieldTypeRef      FieldType = "ref"
)

// IsScalar returns true for types that should be stored in both frontmatter and records.yaml.
func (ft FieldType) IsScalar() bool {
	switch ft {
	case FieldTypeString, FieldTypeInt, FieldTypeFloat, FieldTypeBool,
		FieldTypeDate, FieldTypeDatetime, FieldTypeEnum, FieldTypeRef:
		return true
	default:
		return false
	}
}

// Field defines a single field in a schema.
type Field struct {
	Name      string    `yaml:"-"`
	Type      FieldType `yaml:"type"`
	Required  bool      `yaml:"required"`
	Default   any       `yaml:"default,omitempty"`
	Values    []string  `yaml:"values,omitempty"` // for enum type
	Items     *Field    `yaml:"items,omitempty"`  // for list type
	Fields    FieldMap  `yaml:"fields,omitempty"` // for object type
	RefEntity string    `yaml:"entity,omitempty"` // for ref type: target entity name
}

// FieldMap is an ordered map of field name to Field.
type FieldMap map[string]*Field

// Virtual defines a computed field backed by Starlark.
type Virtual struct {
	Name       string `yaml:"-"`
	Returns    string `yaml:"returns"`               // return type: "string", "int", "list[string]", etc.
	Source     string `yaml:"source"`                // Starlark source code
	Edge       bool   `yaml:"edge,omitempty"`        // if true, returned values create KG edges
	EdgeEntity string `yaml:"edge_entity,omitempty"` // target entity for edges (defaults to same entity)
}

// IsScalarReturn returns true if the virtual's return type is scalar.
func (v *Virtual) IsScalarReturn() bool {
	switch v.Returns {
	case "string", "int", "float", "bool", "date", "datetime":
		return true
	default:
		return false
	}
}

// Schema is the top-level definition of a knowledge base entity.
type Schema struct {
	Version    int                   `yaml:"version"`
	Entity     string                `yaml:"entity"`
	DocsDir    string                `yaml:"docs_dir"`
	Filename   string                `yaml:"filename"`
	RecordsDir string                `yaml:"records_dir"`
	Partition  string                `yaml:"partition"` // "none" or "monthly"
	IDField    string                `yaml:"id_field"`
	DateField  string                `yaml:"date_field"` // field used for monthly partition
	Integrity  string                `yaml:"integrity"`  // "strict", "warn", "off"
	Fields     FieldMap              `yaml:"fields"`
	Virtuals   map[string]*Virtual   `yaml:"virtuals,omitempty"`
	Bucket     string                `yaml:"bucket,omitempty"`      // event bucket; defaults to entity
	EventTypes map[string]*EventType `yaml:"event_types,omitempty"` // events emitted by this entity
}

// EventType describes one verb under this entity's event bucket. The verb
// name is the map key in Schema.EventTypes; concatenated as <bucket>.<verb>
// for the full type name.
type EventType struct {
	Description string                   `yaml:"description,omitempty"`
	Data        *EventDataSchema         `yaml:"data,omitempty"`
	Deprecated  bool                     `yaml:"deprecated,omitempty"`
	Examples    []map[string]interface{} `yaml:"examples,omitempty"`
}

// EventDataSchema describes the shape of an event's `data` payload.
type EventDataSchema struct {
	Fields []*EventDataField `yaml:"fields,omitempty"`
}

// EventDataField is one field under `data`. Type uses the same FieldType
// vocabulary as schema scalar fields (string, int, float, bool, date, enum).
type EventDataField struct {
	Name        string   `yaml:"name"`
	Type        string   `yaml:"type"`
	Required    bool     `yaml:"required,omitempty"`
	EnumValues  []string `yaml:"enum_values,omitempty"`
	MaxLength   int      `yaml:"max_length,omitempty"`
	Pattern     string   `yaml:"pattern,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Deprecated  bool     `yaml:"deprecated,omitempty"`
}

// Validate checks the schema for internal consistency.
func (s *Schema) Validate() error {
	if s.Entity == "" {
		return fmt.Errorf("schema: entity is required")
	}
	if s.DocsDir == "" {
		return fmt.Errorf("schema: docs_dir is required")
	}
	if s.Filename == "" {
		return fmt.Errorf("schema: filename is required")
	}
	if s.IDField == "" {
		s.IDField = "id"
	}
	if s.RecordsDir == "" {
		s.RecordsDir = "data/" + s.Entity
	}
	if s.Partition == "" {
		s.Partition = "none"
	}
	if s.Integrity == "" {
		s.Integrity = "strict"
	}

	if _, ok := s.Fields[s.IDField]; !ok {
		return fmt.Errorf("schema: id_field %q not found in fields", s.IDField)
	}

	if s.Partition == "monthly" && s.DateField == "" {
		return fmt.Errorf("schema: date_field required when partition is monthly")
	}

	for name, f := range s.Fields {
		if err := validateFieldDef(name, f); err != nil {
			return err
		}
	}

	for name, v := range s.Virtuals {
		if v.Returns == "" {
			return fmt.Errorf("schema: virtual %q missing returns type", name)
		}
		if v.Source == "" {
			return fmt.Errorf("schema: virtual %q missing source", name)
		}
	}

	return nil
}

func validateFieldDef(name string, f *Field) error {
	validTypes := map[FieldType]bool{
		FieldTypeString: true, FieldTypeInt: true, FieldTypeFloat: true,
		FieldTypeBool: true, FieldTypeDate: true, FieldTypeDatetime: true,
		FieldTypeEnum: true, FieldTypeList: true, FieldTypeObject: true,
		FieldTypeRef: true,
	}

	if !validTypes[f.Type] {
		return fmt.Errorf("schema: field %q has unknown type %q", name, f.Type)
	}

	if f.Type == FieldTypeEnum && len(f.Values) == 0 {
		return fmt.Errorf("schema: enum field %q must have values", name)
	}

	if f.Type == FieldTypeList && f.Items == nil {
		return fmt.Errorf("schema: list field %q must have items", name)
	}

	if f.Type == FieldTypeObject && len(f.Fields) == 0 {
		return fmt.Errorf("schema: object field %q must have fields", name)
	}

	// Recurse into nested structures
	if f.Items != nil {
		if err := validateFieldDef(name+".items", f.Items); err != nil {
			return err
		}
	}
	for subName, subField := range f.Fields {
		if err := validateFieldDef(name+"."+subName, subField); err != nil {
			return err
		}
	}

	return nil
}

// ScalarFields returns the names of fields that should be stored in records.yaml.
func (s *Schema) ScalarFields() []string {
	var names []string
	for name, f := range s.Fields {
		if f.Type.IsScalar() {
			names = append(names, name)
		}
	}
	// Add scalar virtuals
	for name, v := range s.Virtuals {
		if v.IsScalarReturn() {
			names = append(names, name)
		}
	}
	return names
}

// ComplexFields returns the names of fields that should only be stored in frontmatter.
func (s *Schema) ComplexFields() []string {
	var names []string
	for name, f := range s.Fields {
		if !f.Type.IsScalar() {
			names = append(names, name)
		}
	}
	// Add complex virtuals
	for name, v := range s.Virtuals {
		if !v.IsScalarReturn() {
			names = append(names, name)
		}
	}
	return names
}
