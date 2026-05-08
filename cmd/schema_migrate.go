package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
)

var (
	schemaMigrateCheck   bool
	schemaMigrateInPlace bool
	schemaMigrateOutDir  string
)

var schemaMigrateCmd = &cobra.Command{
	Use:   "migrate <path>...",
	Short: "Rewrite legacy schema files into the JSON Schema 2020-12 + x-* form",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		anyLegacy := false
		for _, p := range args {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			d, err := schema.DetectDialect(data)
			if err != nil {
				return fmt.Errorf("%s: %w", p, err)
			}
			if d != schema.DialectLegacy {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: already new dialect; skipping\n", p)
				continue
			}
			anyLegacy = true
			if schemaMigrateCheck {
				continue
			}
			s, err := schema.ParseLegacy(data)
			if err != nil {
				return fmt.Errorf("%s: parse: %w", p, err)
			}
			out, err := emitNewDialect(s)
			if err != nil {
				return err
			}
			dest := p + ".new.yaml"
			if schemaMigrateInPlace {
				dest = p
			}
			if schemaMigrateOutDir != "" {
				dest = filepath.Join(schemaMigrateOutDir, filepath.Base(p))
			}
			if err := os.WriteFile(dest, out, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", p, dest)
		}
		if schemaMigrateCheck && anyLegacy {
			return fmt.Errorf("legacy schema files present")
		}
		return nil
	},
}

func emitNewDialect(s *schema.Schema) ([]byte, error) {
	doc := map[string]any{
		"$schema":          "https://json-schema.org/draft/2020-12/schema",
		"$id":              "sbdb://" + s.Entity,
		"x-schema-version": s.Version,
		"x-entity":         s.Entity,
		"x-storage":        map[string]any{"docs_dir": s.DocsDir, "filename": s.Filename},
		"x-id":             s.IDField,
		"type":             "object",
	}
	if s.Integrity != "" {
		doc["x-integrity"] = s.Integrity
	}
	if s.Partition != "" && s.Partition != "none" {
		doc["x-partition"] = map[string]any{"mode": s.Partition, "field": s.DateField}
	}
	props := map[string]any{}
	var required []string
	for name, f := range s.Fields {
		props[name] = fieldToJSONSchemaMap(f)
		if f.Required {
			required = append(required, name)
		}
	}
	for name, v := range s.Virtuals {
		m := map[string]any{
			"type":     virtualReturnsToJSONType(v.Returns),
			"readOnly": true,
			"x-compute": map[string]any{
				"source": v.Source,
				"edge":   v.Edge,
			},
		}
		if v.EdgeEntity != "" {
			m["x-compute"].(map[string]any)["edge_entity"] = v.EdgeEntity
		}
		props[name] = m
	}
	doc["properties"] = props
	if len(required) > 0 {
		doc["required"] = required
	}
	return yaml.Marshal(doc)
}

func fieldToJSONSchemaMap(f *schema.Field) map[string]any {
	m := map[string]any{}
	switch f.Type {
	case schema.FieldTypeString:
		m["type"] = "string"
	case schema.FieldTypeInt:
		m["type"] = "integer"
	case schema.FieldTypeFloat:
		m["type"] = "number"
	case schema.FieldTypeBool:
		m["type"] = "boolean"
	case schema.FieldTypeDate:
		m["type"] = "string"
		m["format"] = "date"
	case schema.FieldTypeDatetime:
		m["type"] = "string"
		m["format"] = "date-time"
	case schema.FieldTypeEnum:
		m["enum"] = f.Values
	case schema.FieldTypeList:
		m["type"] = "array"
		if f.Items != nil {
			m["items"] = fieldToJSONSchemaMap(f.Items)
		}
	case schema.FieldTypeObject:
		m["type"] = "object"
		nested := map[string]any{}
		var req []string
		for n, nf := range f.Fields {
			nested[n] = fieldToJSONSchemaMap(nf)
			if nf.Required {
				req = append(req, n)
			}
		}
		m["properties"] = nested
		if len(req) > 0 {
			m["required"] = req
		}
	case schema.FieldTypeRef:
		m["$ref"] = "sbdb://" + f.RefEntity + "#/properties/id"
	}
	if f.Default != nil {
		m["default"] = f.Default
	}
	return m
}

func virtualReturnsToJSONType(returns string) string {
	switch returns {
	case "int":
		return "integer"
	case "float":
		return "number"
	case "bool":
		return "boolean"
	case "date", "datetime":
		return "string"
	case "list", "list[string]", "list[int]":
		return "array"
	default:
		if strings.HasPrefix(returns, "list[") {
			return "array"
		}
		return "string"
	}
}

func init() {
	schemaMigrateCmd.Flags().BoolVar(&schemaMigrateCheck, "check", false, "exit non-zero if any input is legacy")
	schemaMigrateCmd.Flags().BoolVar(&schemaMigrateInPlace, "in-place", false, "rewrite the original file")
	schemaMigrateCmd.Flags().StringVarP(&schemaMigrateOutDir, "out", "o", "", "write migrated files to this directory")
	schemaCmd.AddCommand(schemaMigrateCmd)
}
