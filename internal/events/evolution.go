package events

import (
	"fmt"
	"sort"
)

// FieldSchema mirrors schema.EventDataField but stays inside the events
// package so we don't import schema here. The conversion lives in the
// caller (cmd/event_register.go).
type FieldSchema struct {
	Name       string
	Type       string
	Required   bool
	EnumValues []string
	MaxLength  int
	Pattern    string
	Deprecated bool
}

// TypeSchema describes one type's data-payload shape.
type TypeSchema struct {
	Type   string         // full type name e.g. "x.recipe.cooked"
	Fields []*FieldSchema // ordered for stable diffing
}

// EvolutionResult classifies a schema diff per spec §6.3.
type EvolutionResult struct {
	Allowed         bool
	AddedOptional   []string
	AddedEnumValues map[string][]string // field → values added
	WidenedFields   []string
	ForbiddenReason string
}

// DiffSchema computes the allowed/forbidden classification for a transition
// from `old` → `new` for one type. Implements the matrix in spec §6.3.
//
// Allowed:
//   - new optional field added
//   - enum value appended (additive)
//   - constraint widened (max_length grows, pattern removed)
//   - description / deprecation flag toggled
//
// Forbidden:
//   - field renamed, removed, type-changed
//   - required ↔ optional in either direction
//   - new required field
//   - constraint tightened
func DiffSchema(old, new *TypeSchema) *EvolutionResult {
	r := &EvolutionResult{Allowed: true, AddedEnumValues: map[string][]string{}}

	oldByName := map[string]*FieldSchema{}
	for _, f := range old.Fields {
		oldByName[f.Name] = f
	}
	newByName := map[string]*FieldSchema{}
	for _, f := range new.Fields {
		newByName[f.Name] = f
	}

	// Removed fields → forbidden.
	for name := range oldByName {
		if _, ok := newByName[name]; !ok {
			return forbidden(r, fmt.Sprintf("field %q removed", name))
		}
	}

	// Walk new fields.
	for _, nf := range new.Fields {
		of, existed := oldByName[nf.Name]
		if !existed {
			// Added field — must be optional.
			if nf.Required {
				return forbidden(r, fmt.Sprintf("field %q added as required (must be optional)", nf.Name))
			}
			r.AddedOptional = append(r.AddedOptional, nf.Name)
			continue
		}
		// Field exists in both — check what changed.
		if of.Type != nf.Type {
			return forbidden(r, fmt.Sprintf("field %q type changed: %s → %s", nf.Name, of.Type, nf.Type))
		}
		if of.Required != nf.Required {
			return forbidden(r, fmt.Sprintf(
				"field %q required changed: %v → %v (forbidden in either direction)",
				nf.Name, of.Required, nf.Required))
		}
		// Enum values: additive only.
		oldVals := stringSet(of.EnumValues)
		var addedVals []string
		for _, v := range nf.EnumValues {
			if _, ok := oldVals[v]; !ok {
				addedVals = append(addedVals, v)
			}
		}
		// Removed enum values → forbidden.
		newVals := stringSet(nf.EnumValues)
		for _, v := range of.EnumValues {
			if _, ok := newVals[v]; !ok {
				return forbidden(r, fmt.Sprintf(
					"field %q enum value %q removed (enums are append-only)", nf.Name, v))
			}
		}
		if len(addedVals) > 0 {
			r.AddedEnumValues[nf.Name] = addedVals
		}
		// Constraint widening (max_length grows): allowed.
		// Constraint tightening (max_length shrinks): forbidden.
		if of.MaxLength != 0 && nf.MaxLength != 0 && nf.MaxLength < of.MaxLength {
			return forbidden(r, fmt.Sprintf(
				"field %q max_length tightened: %d → %d", nf.Name, of.MaxLength, nf.MaxLength))
		}
		if of.MaxLength == 0 && nf.MaxLength != 0 {
			// Adding a constraint where there was none = tightening.
			return forbidden(r, fmt.Sprintf(
				"field %q max_length added (constraint tightened)", nf.Name))
		}
		if of.MaxLength != 0 && nf.MaxLength == 0 {
			r.WidenedFields = append(r.WidenedFields, nf.Name+":max_length")
		}
		if of.MaxLength != 0 && nf.MaxLength > of.MaxLength {
			r.WidenedFields = append(r.WidenedFields, nf.Name+":max_length")
		}
		if of.Pattern != nf.Pattern {
			if of.Pattern != "" && nf.Pattern == "" {
				// Pattern removed → loosened.
				r.WidenedFields = append(r.WidenedFields, nf.Name+":pattern")
			} else if of.Pattern == "" && nf.Pattern != "" {
				return forbidden(r, fmt.Sprintf(
					"field %q pattern added (constraint tightened)", nf.Name))
			} else {
				// Pattern changed to a different non-empty value — opaque; treat as tighten.
				return forbidden(r, fmt.Sprintf(
					"field %q pattern changed (cannot mechanically prove widening)", nf.Name))
			}
		}
	}

	sort.Strings(r.AddedOptional)
	sort.Strings(r.WidenedFields)
	return r
}

func forbidden(r *EvolutionResult, reason string) *EvolutionResult {
	r.Allowed = false
	r.ForbiddenReason = reason
	return r
}

func stringSet(s []string) map[string]struct{} {
	m := make(map[string]struct{}, len(s))
	for _, v := range s {
		m[v] = struct{}{}
	}
	return m
}
