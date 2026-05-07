package cmd

import (
	"crypto/rand"
	"errors"
	"os"
	"strings"

	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/acl"
)

var filterCmd = &cobra.Command{
	Use:    "_filter",
	Hidden: true,
	Short:  "Internal: git filter driver entrypoint",
}

var filterCleanCmd = &cobra.Command{
	Use:  "clean <path>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, _ := aclRepoRoot()
		ctx := acl.FilterContext{RepoRoot: root, Random: rand.Reader}
		return acl.FilterClean(cmd.InOrStdin(), cmd.OutOrStdout(), args[0], ctx)
	},
}

var filterSmudgeCmd = &cobra.Command{
	Use:  "smudge <path>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, _ := aclRepoRoot()
		keys, err := loadLocalPrivateKeys(root)
		if err != nil {
			return err
		}
		ctx := acl.FilterContext{RepoRoot: root, PrivateKeys: keys}
		return acl.FilterSmudge(cmd.InOrStdin(), cmd.OutOrStdout(), args[0], ctx)
	},
}

var filterTextconvCmd = &cobra.Command{
	Use:  "textconv <path>",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, _ := aclRepoRoot()
		keys, err := loadLocalPrivateKeys(root)
		if err != nil {
			return err
		}
		ctx := acl.FilterContext{RepoRoot: root, PrivateKeys: keys}
		return acl.FilterTextconv(args[0], cmd.OutOrStdout(), ctx)
	},
}

func loadLocalPrivateKeys(root string) (openpgp.EntityList, error) {
	id, err := acl.LoadIdentity(root)
	if err != nil {
		if errors.Is(err, acl.ErrIdentityMissing) {
			return nil, nil
		}
		return nil, err
	}
	b, err := os.ReadFile(id.PrivateKeyPath)
	if err != nil {
		return nil, err
	}
	return openpgp.ReadArmoredKeyRing(strings.NewReader(string(b)))
}

func init() {
	filterCmd.AddCommand(filterCleanCmd, filterSmudgeCmd, filterTextconvCmd)
	rootCmd.AddCommand(filterCmd)
}
