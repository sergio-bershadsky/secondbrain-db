package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
)

var schemaDiffCmd = &cobra.Command{
	Use:   "diff <old> <new>",
	Short: "Classify deltas between two schemas as additive or breaking",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		oldData, err := readSchemaArg(args[0])
		if err != nil {
			return err
		}
		newData, err := readSchemaArg(args[1])
		if err != nil {
			return err
		}
		oldS, err := schema.Parse(oldData)
		if err != nil {
			return fmt.Errorf("old: %w", err)
		}
		newS, err := schema.Parse(newData)
		if err != nil {
			return fmt.Errorf("new: %w", err)
		}
		report := schema.Diff(oldS, newS)
		fmt.Fprint(cmd.OutOrStdout(), report.String())
		if report.HasBreaking() {
			return fmt.Errorf("breaking changes detected")
		}
		return nil
	},
}

// readSchemaArg supports both filesystem paths and 'HEAD:path' or '<rev>:path'
// syntax that resolves to git-stored content.
func readSchemaArg(arg string) ([]byte, error) {
	if strings.Contains(arg, ":") && !fileExistsAt(arg) {
		out, err := exec.Command("git", "show", arg).Output()
		if err != nil {
			return nil, fmt.Errorf("git show %s: %w", arg, err)
		}
		return out, nil
	}
	return os.ReadFile(arg)
}

func fileExistsAt(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func init() {
	schemaCmd.AddCommand(schemaDiffCmd)
}
