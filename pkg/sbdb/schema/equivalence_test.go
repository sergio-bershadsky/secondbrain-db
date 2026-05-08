package schema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEquivalence_LegacyAndNewProduceSameSchema(t *testing.T) {
	repoRoot := findRepoRoot(t)
	legacyBytes, err := os.ReadFile(filepath.Join(repoRoot, "testdata/schema/legacy/notes.yaml"))
	require.NoError(t, err)
	newBytes, err := os.ReadFile(filepath.Join(repoRoot, "testdata/schema/new/notes.yaml"))
	require.NoError(t, err)

	legacy, err := ParseLegacy(legacyBytes)
	require.NoError(t, err)
	newer, err := ParseJSONSchema(newBytes)
	require.NoError(t, err)

	require.Equal(t, legacy.Entity, newer.Entity)
	require.Equal(t, legacy.DocsDir, newer.DocsDir)
	require.Equal(t, legacy.Filename, newer.Filename)
	require.Equal(t, legacy.IDField, newer.IDField)
	require.Equal(t, legacy.Integrity, newer.Integrity)
	require.Equal(t, len(legacy.Fields), len(newer.Fields))
	for name, lf := range legacy.Fields {
		nf, ok := newer.Fields[name]
		require.True(t, ok, "missing field %s in new dialect", name)
		require.Equal(t, lf.Type, nf.Type, "field %s type mismatch", name)
		require.Equal(t, lf.Required, nf.Required, "field %s required mismatch", name)
		require.Equal(t, lf.RefEntity, nf.RefEntity, "field %s ref entity mismatch", name)
	}
	require.Equal(t, len(legacy.Virtuals), len(newer.Virtuals))
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	for d := wd; d != "/"; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
	}
	t.Fatal("no go.mod above " + wd)
	return ""
}
