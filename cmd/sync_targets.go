package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/sync"
)

var syncTargetsIDFlag string

var syncTargetsCmd = &cobra.Command{
	Use:   "targets",
	Short: "Show resolved sync targets for one doc",
	RunE:  runSyncTargets,
}

func init() {
	syncTargetsCmd.Flags().StringVar(&syncTargetsIDFlag, "id", "", "document id (required)")
	syncCmd.AddCommand(syncTargetsCmd)
}

func runSyncTargets(cmd *cobra.Command, args []string) error {
	if flagSchema == "" {
		return fmt.Errorf("--schema is required")
	}
	if syncTargetsIDFlag == "" {
		return fmt.Errorf("--id is required")
	}
	root := flagBasePath
	if root == "" {
		root, _ = os.Getwd()
	}
	cfgs, err := sync.LoadConfigs(filepath.Join(root, sync.ConfigsDir))
	if err != nil {
		return err
	}
	entityDirs, _, err := discoverEntityDirs(filepath.Join(root, "schemas"))
	if err != nil {
		return err
	}
	dir, ok := entityDirs[flagSchema]
	if !ok {
		return fmt.Errorf("unknown schema %q", flagSchema)
	}
	docPath := filepath.Join(root, dir, syncTargetsIDFlag+".md")

	out := map[string]map[string]interface{}{}
	for _, c := range cfgs {
		binding, applies := c.AppliesTo[flagSchema]
		if !applies {
			continue
		}
		r, err := sync.ResolveDocIntegration(docPath, flagSchema, c)
		if err != nil {
			return err
		}
		out[c.Integration] = map[string]interface{}{
			"target_ref": binding.TargetRef,
			"target_id":  r.TargetID,
			"status":     string(r.Status),
			"required":   binding.Required,
		}
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
