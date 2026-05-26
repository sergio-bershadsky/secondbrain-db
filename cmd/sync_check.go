package cmd

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	yamlV3 "gopkg.in/yaml.v3"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/sync"
)

// ExitCode 4 = drift present; matches the existing doctor exit convention.
const exitDrift = 4

var syncCheckCmd = &cobra.Command{
	Use:           "check",
	Short:         "Report local drift between docs and their last-published state",
	RunE:          runSyncCheck,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	syncCmd.AddCommand(syncCheckCmd)
}

type checkPerInteg struct {
	Result         string `json:"result"`
	TargetID       string `json:"target_id,omitempty"`
	LastPushHash   string `json:"last_push_hash,omitempty"`
	CurrentDocHash string `json:"current_doc_hash,omitempty"`
}

type checkPerDoc struct {
	Schema       string                   `json:"schema"`
	ID           string                   `json:"id"`
	Integrations map[string]checkPerInteg `json:"integrations"`
}

type checkReport struct {
	Docs    []checkPerDoc  `json:"docs"`
	Summary map[string]int `json:"summary"`
}

func runSyncCheck(cmd *cobra.Command, args []string) error {
	root := flagBasePath
	if root == "" {
		root, _ = os.Getwd()
	}
	cfgs, err := sync.LoadConfigs(filepath.Join(root, sync.ConfigsDir))
	if err != nil {
		return err
	}
	if len(cfgs) == 0 {
		return printSyncReport(cmd, checkReport{Docs: []checkPerDoc{}, Summary: map[string]int{}})
	}
	entityDirs, knownEntities, err := discoverEntityDirs(filepath.Join(root, "schemas"))
	if err != nil {
		return err
	}
	for _, c := range cfgs {
		if err := sync.ValidateConfig(c, knownEntities); err != nil {
			return err
		}
	}

	report := checkReport{Summary: map[string]int{}}
	driftFound := false

	for entity, dir := range entityDirs {
		mdFiles, err := walkMD(filepath.Join(root, dir))
		if err != nil {
			return err
		}
		for _, md := range mdFiles {
			id := mdID(md)
			perDoc := checkPerDoc{Schema: entity, ID: id, Integrations: map[string]checkPerInteg{}}
			for _, c := range cfgs {
				if _, applies := c.AppliesTo[entity]; !applies {
					continue
				}
				resolved, err := sync.ResolveDocIntegration(md, entity, c)
				if err != nil {
					return fmt.Errorf("%s [%s]: %w", md, c.Integration, err)
				}
				perInteg := checkPerInteg{
					Result:         resolved.DriftString,
					TargetID:       resolved.TargetID,
					CurrentDocHash: resolved.CurrentDocHash,
				}
				if resolved.State.LastPush != nil {
					perInteg.LastPushHash = resolved.State.LastPush.DocHash
				}
				perDoc.Integrations[c.Integration] = perInteg
				report.Summary[resolved.DriftString]++
				if resolved.DriftResult != sync.ResultInSync {
					driftFound = true
				}
			}
			if len(perDoc.Integrations) > 0 {
				report.Docs = append(report.Docs, perDoc)
			}
		}
	}

	if err := printSyncReport(cmd, report); err != nil {
		return err
	}
	if driftFound {
		return &exitErr{code: exitDrift, msg: "drift present"}
	}
	return nil
}

func printSyncReport(cmd *cobra.Command, r checkReport) error {
	out, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	cmd.OutOrStdout().Write(out)
	cmd.OutOrStdout().Write([]byte("\n"))
	return nil
}

// exitErr is honored by main.go to translate into a specific exit code.
type exitErr struct {
	code int
	msg  string
}

func (e *exitErr) Error() string { return e.msg }
func (e *exitErr) ExitCode() int { return e.code }

func walkMD(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil && os.IsNotExist(err) {
		return nil, nil
	}
	return out, err
}

func mdID(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// discoverEntityDirs reads schemas/*.yaml and returns map[entity]docs_dir
// plus the set of known entity names. Minimal parse — uses generic yaml.
func discoverEntityDirs(schemasDir string) (map[string]string, map[string]bool, error) {
	entries, err := os.ReadDir(schemasDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, map[string]bool{}, nil
		}
		return nil, nil, err
	}
	dirs := map[string]string{}
	known := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(schemasDir, e.Name()))
		if err != nil {
			return nil, nil, err
		}
		var head struct {
			Entity  string `yaml:"x-entity"`
			Storage struct {
				DocsDir string `yaml:"docs_dir"`
			} `yaml:"x-storage"`
		}
		if err := yamlV3.Unmarshal(data, &head); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if head.Entity == "" {
			continue
		}
		known[head.Entity] = true
		if head.Storage.DocsDir != "" {
			dirs[head.Entity] = head.Storage.DocsDir
		} else {
			dirs[head.Entity] = "docs/" + head.Entity
		}
	}
	return dirs, known, nil
}
