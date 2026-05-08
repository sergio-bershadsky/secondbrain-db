package schema

import (
	"fmt"
	"strings"
)

// DiffEntry classifies a single delta between two schemas.
type DiffEntry struct {
	Path    string
	Class   string // "additive" | "breaking"
	Message string
}

// DiffReport aggregates all deltas.
type DiffReport struct {
	Entries []DiffEntry
}

// HasBreaking reports whether any entry is breaking.
func (d DiffReport) HasBreaking() bool {
	for _, e := range d.Entries {
		if e.Class == "breaking" {
			return true
		}
	}
	return false
}

// String prints a human-readable summary.
func (d DiffReport) String() string {
	var b strings.Builder
	for _, e := range d.Entries {
		fmt.Fprintf(&b, "%s %s: %s\n", e.Class, e.Path, e.Message)
	}
	return b.String()
}

// Diff classifies the deltas between old and new schemas.
func Diff(old, newer *Schema) DiffReport {
	var r DiffReport

	if old.DocsDir != newer.DocsDir {
		r.Entries = append(r.Entries, DiffEntry{
			Path: "x-storage.docs_dir", Class: "breaking",
			Message: fmt.Sprintf("%q → %q", old.DocsDir, newer.DocsDir),
		})
	}
	if old.Filename != newer.Filename {
		r.Entries = append(r.Entries, DiffEntry{
			Path: "x-storage.filename", Class: "breaking",
			Message: fmt.Sprintf("%q → %q", old.Filename, newer.Filename),
		})
	}

	for name, of := range old.Fields {
		nf, ok := newer.Fields[name]
		if !ok {
			r.Entries = append(r.Entries, DiffEntry{
				Path: "properties." + name, Class: "breaking", Message: "removed",
			})
			continue
		}
		if of.Type != nf.Type {
			r.Entries = append(r.Entries, DiffEntry{
				Path: "properties." + name + ".type", Class: "breaking",
				Message: fmt.Sprintf("%s → %s", of.Type, nf.Type),
			})
		}
		if !of.Required && nf.Required {
			r.Entries = append(r.Entries, DiffEntry{
				Path: "properties." + name + ".required", Class: "breaking",
				Message: "false → true",
			})
		}
		if of.Required && !nf.Required {
			r.Entries = append(r.Entries, DiffEntry{
				Path: "properties." + name + ".required", Class: "additive",
				Message: "true → false",
			})
		}
		if of.Type == FieldTypeEnum && nf.Type == FieldTypeEnum {
			removed := setDiff(of.Values, nf.Values)
			added := setDiff(nf.Values, of.Values)
			if len(removed) > 0 {
				r.Entries = append(r.Entries, DiffEntry{
					Path: "properties." + name + ".enum", Class: "breaking",
					Message: "removed values: " + strings.Join(removed, ", "),
				})
			}
			if len(added) > 0 {
				r.Entries = append(r.Entries, DiffEntry{
					Path: "properties." + name + ".enum", Class: "additive",
					Message: "added values: " + strings.Join(added, ", "),
				})
			}
		}
	}
	for name, nf := range newer.Fields {
		if _, existed := old.Fields[name]; !existed {
			class := "additive"
			if nf.Required {
				class = "breaking"
			}
			r.Entries = append(r.Entries, DiffEntry{
				Path: "properties." + name, Class: class, Message: "added",
			})
		}
	}
	return r
}

func setDiff(a, b []string) []string {
	bset := map[string]bool{}
	for _, x := range b {
		bset[x] = true
	}
	var out []string
	for _, x := range a {
		if !bset[x] {
			out = append(out, x)
		}
	}
	return out
}
