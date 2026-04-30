package untracked

import (
	"fmt"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/integrity"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/storage"
)

// TamperCheck describes a mismatch for an untracked file.
type TamperCheck struct {
	File       string
	Mismatched []string // "content", "frontmatter"
}

// SignFile computes integrity hashes for a file and returns an Entry.
// If key is non-nil, the entry is HMAC-signed.
func SignFile(basePath, relPath string, key []byte) (*Entry, error) {
	fullPath := basePath + "/" + relPath

	fm, body, err := storage.ParseMarkdown(fullPath)
	if err != nil {
		return nil, fmt.Errorf("reading file for signing: %w", err)
	}

	entry := &Entry{
		File:           relPath,
		ContentSHA:     integrity.HashContent(body),
		FrontmatterSHA: integrity.HashFrontmatter(fm),
	}

	if key != nil {
		intEntry := &integrity.Entry{
			ContentSHA:     entry.ContentSHA,
			FrontmatterSHA: entry.FrontmatterSHA,
			RecordSHA:      "", // no record for untracked files
		}
		entry.Sig = integrity.SignEntry(intEntry, key)
	}

	return entry, nil
}

// VerifyFile checks an untracked file against its registry entry.
// Returns nil if the file matches.
func VerifyFile(entry *Entry, basePath string) *TamperCheck {
	fullPath := basePath + "/" + entry.File

	fm, body, err := storage.ParseMarkdown(fullPath)
	if err != nil {
		return &TamperCheck{File: entry.File, Mismatched: []string{"missing"}}
	}

	var mismatched []string

	currentContentSHA := integrity.HashContent(body)
	if currentContentSHA != entry.ContentSHA {
		mismatched = append(mismatched, "content")
	}

	currentFMSHA := integrity.HashFrontmatter(fm)
	if currentFMSHA != entry.FrontmatterSHA {
		mismatched = append(mismatched, "frontmatter")
	}

	if len(mismatched) == 0 {
		return nil
	}

	return &TamperCheck{File: entry.File, Mismatched: mismatched}
}
