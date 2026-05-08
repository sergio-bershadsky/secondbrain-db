package cmd

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/acl"
)

var aclCmd = &cobra.Command{Use: "acl", Short: "Manage per-document ACLs"}

var aclReadersFlag string

var aclSetCmd = &cobra.Command{
	Use:   "set <doc>",
	Short: "Set the reader list for a document",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, _ := aclRepoRoot()
		kr, err := acl.LoadKeyring(root)
		if err != nil {
			return err
		}
		nicks := splitCSV(aclReadersFlag)
		toks, err := kr.ResolveTokens(nicks)
		if err != nil {
			return err
		}
		aclPath := acl.ACLFileFor(root, args[0])
		return acl.WriteACL(aclPath, acl.ACLFile{Version: 1, Readers: toks})
	},
}

var aclGetCmd = &cobra.Command{
	Use:   "get <doc>",
	Short: "Show the reader list for a document",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, _ := aclRepoRoot()
		kr, err := acl.LoadKeyring(root)
		if err != nil {
			return err
		}
		a, err := acl.ReadACL(acl.ACLFileFor(root, args[0]))
		if err != nil {
			return err
		}
		for _, t := range a.Readers {
			r, ok := kr.ByToken(t)
			if !ok {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s  <unknown>\n", t)
				continue
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  %-10s  %s  %s\n", r.Nickname, t, r.Fingerprint)
		}
		return nil
	},
}

var aclAddCmd = &cobra.Command{
	Use:   "add <doc>",
	Short: "Add reader(s) to a document",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return mutateACL(args[0], func(a *acl.ACLFile, kr *acl.Keyring) error {
			toks, err := kr.ResolveTokens(splitCSV(aclReadersFlag))
			if err != nil {
				return err
			}
			a.Readers = append(a.Readers, toks...)
			return nil
		})
	},
}

var aclRmCmd = &cobra.Command{
	Use:   "rm <doc>",
	Short: "Remove reader(s) from a document",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return mutateACL(args[0], func(a *acl.ACLFile, kr *acl.Keyring) error {
			drop, err := kr.ResolveTokens(splitCSV(aclReadersFlag))
			if err != nil {
				return err
			}
			out := a.Readers[:0]
			for _, t := range a.Readers {
				keep := true
				for _, d := range drop {
					if t == d {
						keep = false
						break
					}
				}
				if keep {
					out = append(out, t)
				}
			}
			a.Readers = out
			return nil
		})
	},
}

var aclLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all ACL'd docs",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, _ := aclRepoRoot()
		dir := acl.ACLDir(root)
		return walkACLs(dir, func(path string) error {
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		})
	},
}

func mutateACL(doc string, f func(*acl.ACLFile, *acl.Keyring) error) error {
	root, _ := aclRepoRoot()
	kr, err := acl.LoadKeyring(root)
	if err != nil {
		return err
	}
	p := acl.ACLFileFor(root, doc)
	a, err := acl.ReadACL(p)
	if err != nil {
		return err
	}
	if err := f(&a, kr); err != nil {
		return err
	}
	return acl.WriteACL(p, a)
}

func splitCSV(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		x = strings.TrimSpace(x)
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

func walkACLs(dir string, fn func(string) error) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(p, ".yaml") {
			return nil
		}
		return fn(p)
	})
}

func init() {
	for _, c := range []*cobra.Command{aclSetCmd, aclAddCmd, aclRmCmd} {
		c.Flags().StringVar(&aclReadersFlag, "readers", "", "comma-separated nicknames")
		_ = c.MarkFlagRequired("readers")
	}
	aclCmd.AddCommand(aclSetCmd, aclGetCmd, aclAddCmd, aclRmCmd, aclLsCmd)
	rootCmd.AddCommand(aclCmd)
}
