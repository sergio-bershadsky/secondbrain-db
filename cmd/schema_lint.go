package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema/meta"
)

var schemaLintCmd = &cobra.Command{
	Use:   "lint <path>...",
	Short: "Validate schema file(s) against the sbdb meta-schema",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := jsonschema.NewCompiler()

		var metaDoc, computeDoc any
		if err := json.Unmarshal(meta.SchemaMeta, &metaDoc); err != nil {
			return err
		}
		if err := json.Unmarshal(meta.ComputeMeta, &computeDoc); err != nil {
			return err
		}
		if err := c.AddResource("https://schemas.sbdb.dev/2026-05/sbdb.compute.schema.json", computeDoc); err != nil {
			return err
		}
		if err := c.AddResource("https://schemas.sbdb.dev/2026-05/sbdb.schema.json", metaDoc); err != nil {
			return err
		}
		metaSchema, err := c.Compile("https://schemas.sbdb.dev/2026-05/sbdb.schema.json")
		if err != nil {
			return fmt.Errorf("compile meta-schema: %w", err)
		}

		var anyErr bool
		for _, p := range args {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			var doc any
			if err := yaml.Unmarshal(data, &doc); err != nil {
				return fmt.Errorf("%s: parse: %w", p, err)
			}
			doc = normaliseForJSON(doc)
			if err := metaSchema.Validate(doc); err != nil {
				fmt.Fprintf(cmd.OutOrStderr(), "%s: %v\n", p, err)
				anyErr = true
				continue
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: ok\n", p)
		}
		if anyErr {
			return fmt.Errorf("lint failed")
		}
		return nil
	},
}

// normaliseForJSON ensures yaml.v3 output (which can include map[any]any
// and similar) is usable by the json-schema validator.
func normaliseForJSON(v any) any {
	switch t := v.(type) {
	case map[any]any:
		m := map[string]any{}
		for k, vv := range t {
			m[fmt.Sprint(k)] = normaliseForJSON(vv)
		}
		return m
	case map[string]any:
		for k, vv := range t {
			t[k] = normaliseForJSON(vv)
		}
		return t
	case []any:
		for i, vv := range t {
			t[i] = normaliseForJSON(vv)
		}
		return t
	default:
		return v
	}
}

func init() {
	schemaCmd.AddCommand(schemaLintCmd)
}
