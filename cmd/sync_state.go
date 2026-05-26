package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/sync"
)

var (
	syncStateIDFlag          string
	syncStateIntegrationFlag string
	// set-mode flags
	syncStatePublishedHash  string
	syncStateRemoteRevision string
	syncStateAt             string
	syncStateActor          string
	syncStateCheckResult    string
	syncStateRemoteRevObs   string
	syncStateError          string
	syncStateErrorStage     string
	syncStateAttemptedHash  string
)

var syncStateCmd = &cobra.Command{
	Use:   "state",
	Short: "Read or write per-doc sync state",
}

var syncStateGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Read the resolved state and payload for one (doc, integration)",
	RunE:  runSyncStateGet,
}

var syncStateSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Record a push, check, or error for one (doc, integration)",
	Long: `Three mutually exclusive modes, chosen by which flags are set:

  Push success:  --published-hash --remote-revision --at [--actor]
  Check result:  --check-result --at [--remote-revision-observed]
  Error:         --error --stage --at [--attempted-doc-hash]
`,
	RunE: runSyncStateSet,
}

func init() {
	for _, c := range []*cobra.Command{syncStateGetCmd, syncStateSetCmd} {
		c.Flags().StringVar(&syncStateIDFlag, "id", "", "document id (required)")
		c.Flags().StringVar(&syncStateIntegrationFlag, "integration", "", "integration name (required)")
	}
	syncStateSetCmd.Flags().StringVar(&syncStatePublishedHash, "published-hash", "", "doc hash that was pushed")
	syncStateSetCmd.Flags().StringVar(&syncStateRemoteRevision, "remote-revision", "", "revision returned by external service")
	syncStateSetCmd.Flags().StringVar(&syncStateAt, "at", "", "RFC3339 timestamp")
	syncStateSetCmd.Flags().StringVar(&syncStateActor, "actor", "claude-code", "actor identifier")
	syncStateSetCmd.Flags().StringVar(&syncStateCheckResult, "check-result", "", "drift result observed")
	syncStateSetCmd.Flags().StringVar(&syncStateRemoteRevObs, "remote-revision-observed", "", "remote revision seen during check")
	syncStateSetCmd.Flags().StringVar(&syncStateError, "error", "", "error message to record")
	syncStateSetCmd.Flags().StringVar(&syncStateErrorStage, "stage", "", "push | check")
	syncStateSetCmd.Flags().StringVar(&syncStateAttemptedHash, "attempted-doc-hash", "", "hash that failed to push")

	syncStateCmd.AddCommand(syncStateGetCmd, syncStateSetCmd)
	syncCmd.AddCommand(syncStateCmd)
}

func requireGetSetCommonFlags() error {
	if flagSchema == "" {
		return fmt.Errorf("--schema is required")
	}
	if syncStateIDFlag == "" {
		return fmt.Errorf("--id is required")
	}
	if syncStateIntegrationFlag == "" {
		return fmt.Errorf("--integration is required")
	}
	return nil
}

func docPathFor(root, entity, id string) (string, error) {
	entityDirs, _, err := discoverEntityDirs(filepath.Join(root, "schemas"))
	if err != nil {
		return "", err
	}
	dir, ok := entityDirs[entity]
	if !ok {
		return "", fmt.Errorf("unknown schema %q", entity)
	}
	return filepath.Join(root, dir, id+".md"), nil
}

func runSyncStateGet(cmd *cobra.Command, args []string) error {
	if err := requireGetSetCommonFlags(); err != nil {
		return err
	}
	root := flagBasePath
	if root == "" {
		root, _ = os.Getwd()
	}
	cfgs, err := sync.LoadConfigs(filepath.Join(root, sync.ConfigsDir))
	if err != nil {
		return err
	}
	cfg, ok := cfgs[syncStateIntegrationFlag]
	if !ok {
		return fmt.Errorf("integration %q not configured", syncStateIntegrationFlag)
	}
	doc, err := docPathFor(root, flagSchema, syncStateIDFlag)
	if err != nil {
		return err
	}
	r, err := sync.ResolveDocIntegration(doc, flagSchema, cfg)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func runSyncStateSet(cmd *cobra.Command, args []string) error {
	if err := requireGetSetCommonFlags(); err != nil {
		return err
	}
	if syncStateAt == "" {
		return fmt.Errorf("--at is required")
	}
	root := flagBasePath
	if root == "" {
		root, _ = os.Getwd()
	}
	doc, err := docPathFor(root, flagSchema, syncStateIDFlag)
	if err != nil {
		return err
	}

	switch {
	case syncStatePublishedHash != "":
		if syncStateRemoteRevision == "" {
			return fmt.Errorf("--remote-revision is required with --published-hash")
		}
		cfgs, _ := sync.LoadConfigs(filepath.Join(root, sync.ConfigsDir))
		cfg := cfgs[syncStateIntegrationFlag]
		r, err := sync.ResolveDocIntegration(doc, flagSchema, cfg)
		if err != nil {
			return err
		}
		return sync.RecordPush(doc, syncStateIntegrationFlag, r.TargetID, sync.PushRecord{
			DocHash:        syncStatePublishedHash,
			At:             syncStateAt,
			RemoteRevision: syncStateRemoteRevision,
			Actor:          syncStateActor,
		})

	case syncStateCheckResult != "":
		if _, err := sync.ParseDriftResult(syncStateCheckResult); err != nil {
			return err
		}
		return sync.RecordCheck(doc, syncStateIntegrationFlag, sync.CheckRecord{
			At:                     syncStateAt,
			Result:                 syncStateCheckResult,
			RemoteRevisionObserved: syncStateRemoteRevObs,
		})

	case syncStateError != "":
		if syncStateErrorStage == "" {
			return fmt.Errorf("--stage is required with --error")
		}
		return sync.RecordError(doc, syncStateIntegrationFlag, sync.ErrorRecord{
			At:               syncStateAt,
			Stage:            syncStateErrorStage,
			Message:          syncStateError,
			AttemptedDocHash: syncStateAttemptedHash,
		})
	}
	return fmt.Errorf("specify one of --published-hash, --check-result, or --error")
}
