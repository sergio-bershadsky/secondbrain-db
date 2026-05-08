package cmd

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/acl"
)

var unlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Register the git filter and decrypt the working tree",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, _ := aclRepoRoot()
		if err := registerFilter(root); err != nil {
			return err
		}
		return ensureGitignore(root)
	},
}

var lockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Re-encrypt the working tree (scrub cleartext)",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, _ := aclRepoRoot()
		dir := acl.ACLDir(root)
		return walkACLs(dir, func(p string) error {
			rel, err := filepath.Rel(acl.ACLDir(root), p)
			if err != nil {
				return err
			}
			docRel := strings.TrimSuffix(rel, ".yaml") + ".md"
			docPath := filepath.Join(root, "docs", docRel)
			in, err := os.ReadFile(docPath)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if acl.IsEnvelopePrefix(in) {
				return nil
			}
			f, err := os.Create(docPath)
			if err != nil {
				return err
			}
			defer f.Close()
			ctx := acl.FilterContext{RepoRoot: root, Random: rand.Reader}
			return acl.FilterClean(bytes.NewReader(in), f, docPath, ctx)
		})
	},
}

func registerFilter(root string) error {
	cfgPath := filepath.Join(root, ".git", "config")
	cfg, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("not a git repo? %w", err)
	}
	additions := `
[filter "sbdb-acl"]
	clean = sbdb _filter clean %f
	smudge = sbdb _filter smudge %f
	required = true
[diff "sbdb-acl"]
	textconv = sbdb _filter textconv %f
`
	if !strings.Contains(string(cfg), `filter "sbdb-acl"`) {
		if err := os.WriteFile(cfgPath, append(cfg, []byte(additions)...), 0o644); err != nil {
			return err
		}
	}
	attrPath := filepath.Join(root, ".gitattributes")
	attrs, err := os.ReadFile(attrPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	line := "docs/**/*.md filter=sbdb-acl diff=sbdb-acl\n"
	if !strings.Contains(string(attrs), "filter=sbdb-acl") {
		if err := os.WriteFile(attrPath, append(attrs, []byte(line)...), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func ensureGitignore(root string) error {
	gi := filepath.Join(root, ".gitignore")
	cur, err := os.ReadFile(gi)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	add := "\n# sbdb local keyring (per-clone, do not commit)\n.sbdb/local-keys/\n.sbdb/local-identity.toml\n.sbdb/local-cache/\n"
	if !strings.Contains(string(cur), ".sbdb/local-keys") {
		if err := os.WriteFile(gi, append(cur, []byte(add)...), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func init() {
	rootCmd.AddCommand(unlockCmd, lockCmd)
}
