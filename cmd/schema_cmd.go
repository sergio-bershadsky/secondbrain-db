package cmd

import (
	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/output"
	"github.com/sergio-bershadsky/secondbrain-db/internal/schema"
)

var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Schema introspection and management",
}

var schemaShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display the active schema",
	RunE:  runSchemaShow,
}

var schemaListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available schemas",
	RunE:  runSchemaList,
}

var schemaJSONCmd = &cobra.Command{
	Use:   "json-schema",
	Short: "Emit JSON Schema for the record shape",
	RunE:  runSchemaJSON,
}

func init() {
	schemaCmd.AddCommand(schemaShowCmd)
	schemaCmd.AddCommand(schemaListCmd)
	schemaCmd.AddCommand(schemaJSONCmd)
	rootCmd.AddCommand(schemaCmd)
}

func runSchemaShow(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	s, err := loadSchema(cfg)
	if err != nil {
		return err
	}

	format := outputFormat(cfg)

	// Build schema info
	scalarFields, complexFields := schema.ClassifyFields(s)
	var virtualInfo []map[string]any
	for name, v := range s.Virtuals {
		virtualInfo = append(virtualInfo, map[string]any{
			"name":    name,
			"returns": v.Returns,
			"scalar":  v.IsScalarReturn(),
		})
	}

	result := map[string]any{
		"entity":         s.Entity,
		"docs_dir":       s.DocsDir,
		"filename":       s.Filename,
		"records_dir":    s.RecordsDir,
		"partition":      s.Partition,
		"id_field":       s.IDField,
		"integrity":      s.Integrity,
		"scalar_fields":  scalarFields,
		"complex_fields": complexFields,
		"virtuals":       virtualInfo,
	}

	return output.PrintData(format, result)
}

func runSchemaList(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	format := outputFormat(cfg)

	names, err := schema.ListSchemas(cfg.SchemaDir)
	if err != nil {
		return err
	}

	return output.PrintData(format, map[string]any{
		"schema_dir": cfg.SchemaDir,
		"schemas":    names,
	})
}

func runSchemaJSON(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	s, err := loadSchema(cfg)
	if err != nil {
		return err
	}

	format := outputFormat(cfg)

	// Build a JSON Schema from the YAML schema
	jsonSchema := buildJSONSchema(s)
	return output.PrintData(format, jsonSchema)
}

func buildJSONSchema(s *schema.Schema) map[string]any {
	properties := make(map[string]any)
	var required []string

	for name, f := range s.Fields {
		prop := fieldToJSONSchema(f)
		properties[name] = prop
		if f.Required {
			required = append(required, name)
		}
	}

	return map[string]any{
		"$schema":    "https://json-schema.org/draft/2020-12/schema",
		"title":      s.Entity,
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}

func fieldToJSONSchema(f *schema.Field) map[string]any {
	prop := make(map[string]any)

	switch f.Type {
	case schema.FieldTypeString:
		prop["type"] = "string"
	case schema.FieldTypeInt:
		prop["type"] = "integer"
	case schema.FieldTypeFloat:
		prop["type"] = "number"
	case schema.FieldTypeBool:
		prop["type"] = "boolean"
	case schema.FieldTypeDate:
		prop["type"] = "string"
		prop["format"] = "date"
	case schema.FieldTypeDatetime:
		prop["type"] = "string"
		prop["format"] = "date-time"
	case schema.FieldTypeEnum:
		prop["type"] = "string"
		enum := make([]any, len(f.Values))
		for i, v := range f.Values {
			enum[i] = v
		}
		prop["enum"] = enum
	case schema.FieldTypeList:
		prop["type"] = "array"
		if f.Items != nil {
			prop["items"] = fieldToJSONSchema(f.Items)
		}
	case schema.FieldTypeObject:
		prop["type"] = "object"
		if len(f.Fields) > 0 {
			subProps := make(map[string]any)
			for name, sf := range f.Fields {
				subProps[name] = fieldToJSONSchema(sf)
			}
			prop["properties"] = subProps
		}
	}

	if f.Default != nil {
		prop["default"] = f.Default
	}

	return prop
}
