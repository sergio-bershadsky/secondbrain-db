package cmd

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp/armor"
	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/acl"
)

var keysCmd = &cobra.Command{Use: "keys", Short: "Manage ACL keys and identities"}

var (
	keysSelfInitName     string
	keysSelfInitEmail    string
	keysSelfInitNickname string
	keysExportOut        string
)

var keysSelfInitCmd = &cobra.Command{
	Use:   "self-init",
	Short: "Generate a new identity for this clone (and a fresh keypair)",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := aclRepoRoot()
		if err != nil {
			return err
		}
		entity, err := openpgp.NewEntity(keysSelfInitName, "", keysSelfInitEmail, nil)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(acl.LocalKeysDir(root), 0o700); err != nil {
			return err
		}
		privPath := filepath.Join(acl.LocalKeysDir(root), "private.asc")
		f, err := os.OpenFile(privPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		w, err := armor.Encode(f, "PGP PRIVATE KEY BLOCK", nil)
		if err != nil {
			f.Close()
			return err
		}
		if err := entity.SerializePrivateWithoutSigning(w, nil); err != nil {
			w.Close()
			f.Close()
			return err
		}
		w.Close()
		f.Close()

		tok, err := acl.NewToken()
		if err != nil {
			return err
		}
		fingerprint := strings.ToUpper(fmt.Sprintf("%x", entity.PrimaryKey.Fingerprint))
		id := acl.Identity{
			Nickname:       keysSelfInitNickname,
			Fingerprint:    fingerprint,
			PrivateKeyPath: privPath,
		}
		if err := acl.SaveIdentity(root, id); err != nil {
			return err
		}

		kr, err := acl.LoadKeyring(root)
		if err != nil {
			return err
		}
		kr.SetRoot(root)
		var pubBuf strings.Builder
		armW, _ := armor.Encode(&pubBuf, "PGP PUBLIC KEY BLOCK", nil)
		if err := entity.Serialize(armW); err != nil {
			return err
		}
		armW.Close()

		if err := os.MkdirAll(acl.LocalPubkeysDir(root), 0o755); err != nil {
			return err
		}
		pubFile := "pubkeys/" + fingerprint + ".asc"
		if err := os.WriteFile(filepath.Join(acl.LocalPubkeysDir(root), fingerprint+".asc"), []byte(pubBuf.String()), 0o644); err != nil {
			return err
		}
		kr.Recipients = append(kr.Recipients, acl.Recipient{
			Nickname:    id.Nickname,
			Token:       tok,
			Fingerprint: fingerprint,
			Name:        keysSelfInitName,
			Email:       keysSelfInitEmail,
			PubkeyFile:  pubFile,
		})
		if err := kr.Save(); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "identity created: %s (token=%s)\n", id.Nickname, tok)
		return nil
	},
}

var keysExportCmd = &cobra.Command{
	Use:   "export <nickname>",
	Short: "Export a shareable identity bundle",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := aclRepoRoot()
		if err != nil {
			return err
		}
		kr, err := acl.LoadKeyring(root)
		if err != nil {
			return err
		}
		r, ok := kr.ByNickname(args[0])
		if !ok {
			return fmt.Errorf("unknown nickname %q", args[0])
		}
		id, err := acl.LoadIdentity(root)
		if err != nil {
			return err
		}
		privBytes, err := os.ReadFile(id.PrivateKeyPath)
		if err != nil {
			return err
		}
		privEntities, err := openpgp.ReadArmoredKeyRing(strings.NewReader(string(privBytes)))
		if err != nil {
			return err
		}
		if len(privEntities) == 0 {
			return fmt.Errorf("no private key in %s", id.PrivateKeyPath)
		}
		bundleBytes, err := acl.ExportBundle(acl.BundleExportOpts{
			Nickname: r.Nickname, Token: r.Token, Entity: privEntities[0], Random: rand.Reader,
		})
		if err != nil {
			return err
		}
		dest := keysExportOut
		if dest == "" {
			dest = r.Nickname + ".bundle.yaml"
		}
		return os.WriteFile(dest, bundleBytes, 0o644)
	},
}

var keysImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import a peer's identity bundle",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := aclRepoRoot()
		if err != nil {
			return err
		}
		b, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		bundle, err := acl.ImportBundle(b)
		if err != nil {
			return err
		}
		kr, err := acl.LoadKeyring(root)
		if err != nil {
			return err
		}
		kr.SetRoot(root)
		if err := os.MkdirAll(acl.LocalPubkeysDir(root), 0o755); err != nil {
			return err
		}
		pubFile := "pubkeys/" + bundle.Fingerprint + ".asc"
		if err := os.WriteFile(filepath.Join(acl.LocalPubkeysDir(root), bundle.Fingerprint+".asc"), []byte(bundle.Pubkey), 0o644); err != nil {
			return err
		}
		kr.Recipients = append(kr.Recipients, acl.Recipient{
			Nickname:    bundle.Nickname,
			Token:       bundle.Token,
			Fingerprint: bundle.Fingerprint,
			Name:        bundle.Name,
			Email:       bundle.Email,
			PubkeyFile:  pubFile,
		})
		if err := kr.Save(); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "imported: %s (%s)\n", bundle.Nickname, bundle.Token)
		return nil
	},
}

var keysListCmd = &cobra.Command{
	Use:   "list",
	Short: "List local recipients",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := aclRepoRoot()
		if err != nil {
			return err
		}
		kr, err := acl.LoadKeyring(root)
		if err != nil {
			return err
		}
		for _, r := range kr.Recipients {
			fmt.Fprintf(cmd.OutOrStdout(), "%-12s %s  %s <%s>\n", r.Nickname, r.Token, r.Name, r.Email)
		}
		return nil
	},
}

var keysWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show this clone's local identity",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := aclRepoRoot()
		if err != nil {
			return err
		}
		id, err := acl.LoadIdentity(root)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "nickname=%s fingerprint=%s key=%s\n", id.Nickname, id.Fingerprint, id.PrivateKeyPath)
		return nil
	},
}

func init() {
	keysSelfInitCmd.Flags().StringVar(&keysSelfInitName, "name", "", "full name")
	keysSelfInitCmd.Flags().StringVar(&keysSelfInitEmail, "email", "", "email")
	keysSelfInitCmd.Flags().StringVar(&keysSelfInitNickname, "nickname", "", "nickname for this identity")
	keysExportCmd.Flags().StringVarP(&keysExportOut, "out", "o", "", "output file (default <nickname>.bundle.yaml)")
	keysCmd.AddCommand(keysSelfInitCmd, keysExportCmd, keysImportCmd, keysListCmd, keysWhoamiCmd)
	rootCmd.AddCommand(keysCmd)
}

// aclRepoRoot returns the working directory; once we have config-driven
// roots elsewhere this will defer to that. For now CWD is the repo root.
func aclRepoRoot() (string, error) { return os.Getwd() }
