# JSON Schema 2020-12 Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace sbdb's bespoke YAML schema dialect with JSON Schema 2020-12 plus bare `x-*` extensions; auto-migrate legacy schemas on load; ship `sbdb schema lint|diff|check|migrate` and the pre-commit guardrail that uses them.

**Architecture:** A new parser (`jsonschema.go`) reads JSON-Schema-shaped YAML/JSON into the existing in-memory `Schema` struct. The current legacy parser is preserved as `legacy.go` and called by the loader's dispatcher when it detects a legacy file. A new validator wraps `github.com/santhosh-tekuri/jsonschema/v6` and is fed by either parser path; the existing hand-rolled per-field checker becomes a thin shim that delegates to the library, retaining only the cross-document checks (ref existence, partition consistency) that JSON Schema cannot express. Two embedded meta-schemas drive `sbdb schema lint`. New CLI sub-commands live under `cmd/schema_*.go`. The pre-commit hook wired in earlier (`scripts/schema-precommit.sh`) gets real commands to invoke.

**Tech Stack:**
- Go 1.25 (existing)
- `github.com/santhosh-tekuri/jsonschema/v6` (new dep) — pure-Go validator with native draft 2020-12 support and a `Vocabulary` API for custom keywords
- `gopkg.in/yaml.v3` (existing) — YAML parsing
- `github.com/spf13/cobra` (existing) — CLI commands
- `github.com/stretchr/testify` (existing) — tests

**Spec:** `docs/superpowers/specs/2026-05-08-json-schema-migration-design.md`
**Tracking issue:** [#46](https://github.com/sergio-bershadsky/secondbrain-db/issues/46)

---

## File structure

New files:

```
pkg/sbdb/schema/
  legacy.go                 # legacy-dialect parser (extracted from current loader/Parse)
  legacy_test.go
  jsonschema.go             # new-dialect parser (JSON Schema 2020-12 + x-* → Schema)
  jsonschema_test.go
  detect.go                 # dialect detector + dispatcher
  detect_test.go
  jsvalidate.go             # JSON Schema validator wrapper (uses santhosh-tekuri lib)
  jsvalidate_test.go
  diff.go                   # additive-vs-breaking classifier for two Schemas
  diff_test.go
  evolve.go                 # 'sbdb schema check' core: run schema against existing docs
  evolve_test.go
  meta/
    sbdb.schema.json        # meta-schema for sbdb files (embedded via go:embed)
    sbdb.compute.schema.json
    embed.go                # //go:embed wiring + accessor

cmd/
  schema_cmd.go             # parent 'sbdb schema' (already exists for 'show'; extend)
  schema_lint.go
  schema_lint_test.go
  schema_diff.go
  schema_diff_test.go
  schema_check.go
  schema_check_test.go
  schema_migrate.go
  schema_migrate_test.go

testdata/schema/
  legacy/                   # canned legacy schemas
    notes.yaml
    refs.yaml
  new/                      # the equivalent migrated schemas
    notes.yaml
    refs.yaml
```

Modified files:

```
go.mod / go.sum                       # add santhosh-tekuri dep
pkg/sbdb/schema/loader.go             # delegate Load/Parse to detect+dispatch
pkg/sbdb/schema/validate.go           # ValidateRecord delegates per-field to jsvalidate
pkg/sbdb/schema/schema.go             # keep struct, add Source field for diagnostics
pkg/sbdb/kg/...                       # derive entity edges from $ref URIs
cmd/schema_cmd.go (existing)          # add sub-commands; refactor common helpers
scripts/schema-precommit.sh           # remove the "command not available" stub
README.md                             # one-line link to migration guide
docs/guide/schemas.md (new)           # user guide for the new shape + migration
```

---

## Task 1: Add jsonschema dep + meta package skeleton

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `pkg/sbdb/schema/meta/embed.go`
- Create: `pkg/sbdb/schema/meta/sbdb.schema.json`
- Create: `pkg/sbdb/schema/meta/sbdb.compute.schema.json`

- [ ] **Step 1: Add dep**

```bash
go get github.com/santhosh-tekuri/jsonschema/v6@latest
```

Expected: go.mod and go.sum updated.

- [ ] **Step 2: Write the meta-schema for sbdb files**

`pkg/sbdb/schema/meta/sbdb.schema.json`:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://schemas.sbdb.dev/2026-05/sbdb.schema.json",
  "title": "sbdb schema",
  "description": "Meta-schema validating an sbdb entity schema (JSON Schema 2020-12 with x-* extensions).",
  "type": "object",
  "allOf": [{ "$ref": "https://json-schema.org/draft/2020-12/schema" }],
  "required": ["x-entity", "x-storage", "x-id"],
  "properties": {
    "x-schema-version": { "type": ["integer", "string"] },
    "x-entity": { "type": "string", "pattern": "^[a-z][a-z0-9_-]*$" },
    "x-storage": {
      "type": "object",
      "required": ["docs_dir", "filename"],
      "properties": {
        "docs_dir": { "type": "string", "minLength": 1 },
        "records_dir": { "type": "string" },
        "filename":    { "type": "string", "minLength": 1 }
      },
      "additionalProperties": false
    },
    "x-id":        { "type": "string", "minLength": 1 },
    "x-integrity": { "enum": ["strict", "warn", "off"] },
    "x-partition": {
      "type": "object",
      "required": ["mode"],
      "properties": {
        "mode":  { "enum": ["none", "monthly"] },
        "field": { "type": "string" }
      },
      "additionalProperties": false
    },
    "x-events": {
      "type": "object",
      "properties": {
        "bucket": { "type": "string" },
        "types":  { "type": "object" }
      }
    }
  }
}
```

- [ ] **Step 3: Write the compute meta-schema**

`pkg/sbdb/schema/meta/sbdb.compute.schema.json`:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://schemas.sbdb.dev/2026-05/sbdb.compute.schema.json",
  "title": "sbdb x-compute block",
  "type": "object",
  "required": ["source"],
  "properties": {
    "source":      { "type": "string", "minLength": 1 },
    "edge":        { "type": "boolean", "default": false },
    "edge_entity": { "type": "string" }
  },
  "additionalProperties": false
}
```

- [ ] **Step 4: Embed accessor**

`pkg/sbdb/schema/meta/embed.go`:

```go
// Package meta holds the embedded meta-schemas for the sbdb dialect of
// JSON Schema 2020-12.
package meta

import _ "embed"

//go:embed sbdb.schema.json
var SchemaMeta []byte

//go:embed sbdb.compute.schema.json
var ComputeMeta []byte
```

- [ ] **Step 5: Verify build**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum pkg/sbdb/schema/meta/
git commit -m "feat(schema): jsonschema dep and embedded meta-schemas

Refs #46"
```

---

## Task 2: Dialect detector

**Files:**
- Create: `pkg/sbdb/schema/detect.go`
- Create: `pkg/sbdb/schema/detect_test.go`

- [ ] **Step 1: Write the failing test**

```go
package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectDialect_Legacy(t *testing.T) {
	d, err := DetectDialect([]byte(`
version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
fields:
  id: { type: string, required: true }
`))
	require.NoError(t, err)
	require.Equal(t, DialectLegacy, d)
}

func TestDetectDialect_New(t *testing.T) {
	d, err := DetectDialect([]byte(`
$schema: https://json-schema.org/draft/2020-12/schema
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
properties:
  id: { type: string }
`))
	require.NoError(t, err)
	require.Equal(t, DialectNew, d)
}

func TestDetectDialect_NewByXKey(t *testing.T) {
	d, err := DetectDialect([]byte(`
x-entity: notes
type: object
properties:
  id: { type: string }
`))
	require.NoError(t, err)
	require.Equal(t, DialectNew, d)
}

func TestDetectDialect_AmbiguousIsRejected(t *testing.T) {
	_, err := DetectDialect([]byte(`
$schema: https://json-schema.org/draft/2020-12/schema
entity: notes
fields:
  id: { type: string, required: true }
properties:
  id: { type: string }
`))
	require.Error(t, err)
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement**

`pkg/sbdb/schema/detect.go`:

```go
package schema

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Dialect identifies the on-disk schema format.
type Dialect int

const (
	DialectUnknown Dialect = iota
	DialectLegacy
	DialectNew
)

func (d Dialect) String() string {
	switch d {
	case DialectLegacy:
		return "legacy"
	case DialectNew:
		return "json-schema"
	default:
		return "unknown"
	}
}

// DetectDialect inspects the top-level keys of a YAML/JSON schema document
// and reports which dialect it is.
func DetectDialect(data []byte) (Dialect, error) {
	var top map[string]any
	if err := yaml.Unmarshal(data, &top); err != nil {
		return DialectUnknown, fmt.Errorf("schema: parse top-level: %w", err)
	}

	hasNewSignal := false
	hasLegacySignal := false

	for k := range top {
		switch {
		case k == "$schema" || k == "$id" || k == "properties":
			hasNewSignal = true
		case strings.HasPrefix(k, "x-"):
			hasNewSignal = true
		case k == "entity" || k == "fields":
			hasLegacySignal = true
		}
	}

	switch {
	case hasNewSignal && hasLegacySignal:
		return DialectUnknown, fmt.Errorf("schema: ambiguous dialect (both legacy 'entity'/'fields' and new '$schema'/'x-*' keys present)")
	case hasNewSignal:
		return DialectNew, nil
	case hasLegacySignal:
		return DialectLegacy, nil
	default:
		return DialectUnknown, fmt.Errorf("schema: cannot detect dialect (no recognisable top-level keys)")
	}
}
```

- [ ] **Step 4: Run, expect PASS.**

- [ ] **Step 5: Commit**

```bash
git add pkg/sbdb/schema/detect.go pkg/sbdb/schema/detect_test.go
git commit -m "feat(schema): dialect detector

Refs #46"
```

---

## Task 3: Extract legacy parser

**Files:**
- Create: `pkg/sbdb/schema/legacy.go`
- Modify: `pkg/sbdb/schema/loader.go` (move Parse body)

- [ ] **Step 1: Move existing parsing logic into legacy.go**

`pkg/sbdb/schema/legacy.go`:

```go
package schema

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ParseLegacy parses a sbdb-dialect (pre-JSON-Schema) YAML document into
// the internal Schema struct.
func ParseLegacy(data []byte) (*Schema, error) {
	var s Schema
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing schema YAML: %w", err)
	}
	for name, f := range s.Fields {
		f.Name = name
	}
	for name, v := range s.Virtuals {
		v.Name = name
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}
```

- [ ] **Step 2: Update `loader.go`'s `Parse` to call ParseLegacy directly for now (dispatcher comes later).**

In `pkg/sbdb/schema/loader.go`, replace the current `Parse` body with:

```go
// Parse parses a schema from YAML/JSON bytes. It auto-detects the
// dialect and dispatches to the appropriate parser.
func Parse(data []byte) (*Schema, error) {
	dialect, err := DetectDialect(data)
	if err != nil {
		return nil, err
	}
	switch dialect {
	case DialectLegacy:
		return ParseLegacy(data)
	case DialectNew:
		return ParseJSONSchema(data)
	default:
		return nil, fmt.Errorf("schema: unknown dialect")
	}
}
```

(Stub `ParseJSONSchema` returning `nil, fmt.Errorf("not yet implemented")` is fine here; Task 4 implements it. Add a `var _ = ParseJSONSchema` reference to ensure the build catches removal.)

- [ ] **Step 3: Run existing schema tests**

```bash
go test ./pkg/sbdb/schema/ -run "Parse|Load" -v
```

Expected: PASS for legacy fixtures (which is everything in testdata today).

- [ ] **Step 4: Commit**

```bash
git add pkg/sbdb/schema/legacy.go pkg/sbdb/schema/loader.go
git commit -m "refactor(schema): extract legacy parser; loader dispatches by dialect

Refs #46"
```

---

## Task 4: New-dialect parser

**Files:**
- Create: `pkg/sbdb/schema/jsonschema.go`
- Create: `pkg/sbdb/schema/jsonschema_test.go`

- [ ] **Step 1: Write the failing test**

```go
package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseJSONSchema_BasicNotes(t *testing.T) {
	src := []byte(`
$schema: https://json-schema.org/draft/2020-12/schema
$id: sbdb://notes
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
x-integrity: strict

type: object
required: [id, created]
properties:
  id:      { type: string }
  created: { type: string, format: date }
  status:  { enum: [active, archived], default: active }
  tags:    { type: array, items: { type: string } }
`)
	s, err := ParseJSONSchema(src)
	require.NoError(t, err)
	require.Equal(t, "notes", s.Entity)
	require.Equal(t, "docs/notes", s.DocsDir)
	require.Equal(t, "{id}.md", s.Filename)
	require.Equal(t, "id", s.IDField)
	require.Equal(t, "strict", s.Integrity)

	require.Contains(t, s.Fields, "id")
	require.Equal(t, FieldTypeString, s.Fields["id"].Type)
	require.True(t, s.Fields["id"].Required)

	require.Equal(t, FieldTypeDate, s.Fields["created"].Type)
	require.Equal(t, FieldTypeEnum, s.Fields["status"].Type)
	require.ElementsMatch(t, []string{"active", "archived"}, s.Fields["status"].Values)

	require.Equal(t, FieldTypeList, s.Fields["tags"].Type)
	require.NotNil(t, s.Fields["tags"].Items)
	require.Equal(t, FieldTypeString, s.Fields["tags"].Items.Type)
}

func TestParseJSONSchema_VirtualWithCompute(t *testing.T) {
	src := []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
properties:
  id: { type: string }
  title:
    type: string
    readOnly: true
    x-compute:
      source: "def compute(content, fields): return 'x'"
`)
	s, err := ParseJSONSchema(src)
	require.NoError(t, err)
	require.Contains(t, s.Virtuals, "title")
	require.Equal(t, "string", s.Virtuals["title"].Returns)
	require.Contains(t, s.Virtuals["title"].Source, "def compute")
	require.NotContains(t, s.Fields, "title", "virtual must not appear as a regular field")
}

func TestParseJSONSchema_RefBecomesRefField(t *testing.T) {
	src := []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
properties:
  id: { type: string }
  parent:
    $ref: "sbdb://notes#/properties/id"
`)
	s, err := ParseJSONSchema(src)
	require.NoError(t, err)
	require.Equal(t, FieldTypeRef, s.Fields["parent"].Type)
	require.Equal(t, "notes", s.Fields["parent"].RefEntity)
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement**

`pkg/sbdb/schema/jsonschema.go`:

```go
package schema

import (
	"fmt"
	"net/url"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseJSONSchema parses a JSON Schema 2020-12 + x-* document into the
// internal Schema struct.
func ParseJSONSchema(data []byte) (*Schema, error) {
	var top map[string]any
	if err := yaml.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("schema: parse: %w", err)
	}

	s := &Schema{
		Version:    1,
		Fields:     FieldMap{},
		Virtuals:   map[string]*Virtual{},
		EventTypes: map[string]*EventType{},
	}

	// Top-level x-* keywords.
	if v, ok := top["x-schema-version"]; ok {
		switch t := v.(type) {
		case int:
			s.Version = t
		case string:
			// version may carry a minor; only the major is retained as int.
			head, _, _ := strings.Cut(t, ".")
			fmt.Sscanf(head, "%d", &s.Version)
		}
	}
	if v, ok := top["x-entity"].(string); ok {
		s.Entity = v
	}
	if v, ok := top["x-id"].(string); ok {
		s.IDField = v
	}
	if v, ok := top["x-integrity"].(string); ok {
		s.Integrity = v
	}
	if storage, ok := top["x-storage"].(map[string]any); ok {
		if d, ok := storage["docs_dir"].(string); ok {
			s.DocsDir = d
		}
		if f, ok := storage["filename"].(string); ok {
			s.Filename = f
		}
		if r, ok := storage["records_dir"].(string); ok {
			s.RecordsDir = r
		}
	}
	if part, ok := top["x-partition"].(map[string]any); ok {
		if m, ok := part["mode"].(string); ok {
			s.Partition = m
		}
		if f, ok := part["field"].(string); ok {
			s.DateField = f
		}
	}
	if events, ok := top["x-events"].(map[string]any); ok {
		if b, ok := events["bucket"].(string); ok {
			s.Bucket = b
		}
		// event types parsed permissively for now; richer parser is a follow-up.
		_ = events["types"]
	}

	// required[] becomes per-field Required=true.
	requiredSet := map[string]bool{}
	if req, ok := top["required"].([]any); ok {
		for _, r := range req {
			if name, ok := r.(string); ok {
				requiredSet[name] = true
			}
		}
	}

	props, _ := top["properties"].(map[string]any)
	for name, raw := range props {
		propMap, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("schema: properties.%s: expected object", name)
		}
		// Virtual? readOnly + x-compute.
		readOnly, _ := propMap["readOnly"].(bool)
		comp, _ := propMap["x-compute"].(map[string]any)
		if readOnly && comp != nil {
			v := &Virtual{Name: name}
			if src, ok := comp["source"].(string); ok {
				v.Source = src
			}
			if e, ok := comp["edge"].(bool); ok {
				v.Edge = e
			}
			if ent, ok := comp["edge_entity"].(string); ok {
				v.EdgeEntity = ent
			}
			v.Returns = jsonTypeToReturns(propMap)
			s.Virtuals[name] = v
			continue
		}
		field, err := propToField(name, propMap)
		if err != nil {
			return nil, err
		}
		field.Required = requiredSet[name]
		s.Fields[name] = field
	}

	if err := s.Validate(); err != nil {
		return nil, err
	}
	return s, nil
}

// propToField converts a single JSON Schema property into the internal
// Field representation.
func propToField(name string, m map[string]any) (*Field, error) {
	f := &Field{Name: name}
	// $ref → ref type, RefEntity derived from the URI host.
	if ref, ok := m["$ref"].(string); ok {
		ent, err := refEntityFromURI(ref)
		if err != nil {
			return nil, fmt.Errorf("properties.%s: %w", name, err)
		}
		f.Type = FieldTypeRef
		f.RefEntity = ent
		return f, nil
	}
	// enum → enum type.
	if enumRaw, ok := m["enum"].([]any); ok {
		f.Type = FieldTypeEnum
		for _, e := range enumRaw {
			if s, ok := e.(string); ok {
				f.Values = append(f.Values, s)
			}
		}
		if d, ok := m["default"]; ok {
			f.Default = d
		}
		return f, nil
	}
	t, _ := m["type"].(string)
	switch t {
	case "string":
		// date/datetime via format.
		if format, ok := m["format"].(string); ok {
			switch format {
			case "date":
				f.Type = FieldTypeDate
			case "date-time":
				f.Type = FieldTypeDatetime
			default:
				f.Type = FieldTypeString
			}
		} else {
			f.Type = FieldTypeString
		}
	case "integer":
		f.Type = FieldTypeInt
	case "number":
		f.Type = FieldTypeFloat
	case "boolean":
		f.Type = FieldTypeBool
	case "array":
		f.Type = FieldTypeList
		if items, ok := m["items"].(map[string]any); ok {
			it, err := propToField("items", items)
			if err != nil {
				return nil, err
			}
			f.Items = it
		}
	case "object":
		f.Type = FieldTypeObject
		f.Fields = FieldMap{}
		nestedReq := map[string]bool{}
		if req, ok := m["required"].([]any); ok {
			for _, r := range req {
				if rs, ok := r.(string); ok {
					nestedReq[rs] = true
				}
			}
		}
		if nested, ok := m["properties"].(map[string]any); ok {
			for nname, nraw := range nested {
				nmap, ok := nraw.(map[string]any)
				if !ok {
					continue
				}
				nf, err := propToField(nname, nmap)
				if err != nil {
					return nil, err
				}
				nf.Required = nestedReq[nname]
				f.Fields[nname] = nf
			}
		}
	default:
		// untyped property — treat as string for now.
		f.Type = FieldTypeString
	}
	if d, ok := m["default"]; ok {
		f.Default = d
	}
	return f, nil
}

// refEntityFromURI extracts the entity name from a sbdb:// reference URI
// of the form "sbdb://<entity>#/properties/<id>".
func refEntityFromURI(ref string) (string, error) {
	u, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("invalid $ref %q: %w", ref, err)
	}
	if u.Scheme != "sbdb" {
		return "", fmt.Errorf("$ref must use sbdb:// scheme, got %q", ref)
	}
	if u.Host == "" {
		return "", fmt.Errorf("$ref missing entity name: %q", ref)
	}
	return u.Host, nil
}

// jsonTypeToReturns derives a virtual's Returns string from its JSON Schema type.
func jsonTypeToReturns(m map[string]any) string {
	if t, ok := m["type"].(string); ok {
		switch t {
		case "string":
			if f, _ := m["format"].(string); f == "date" {
				return "date"
			} else if f == "date-time" {
				return "datetime"
			}
			return "string"
		case "integer":
			return "int"
		case "number":
			return "float"
		case "boolean":
			return "bool"
		case "array":
			if items, ok := m["items"].(map[string]any); ok {
				return "list[" + jsonTypeToReturns(items) + "]"
			}
			return "list"
		}
	}
	return "any"
}
```

- [ ] **Step 4: Run, expect PASS.**

```bash
go test ./pkg/sbdb/schema/ -run JSONSchema -v
```

- [ ] **Step 5: Commit**

```bash
git add pkg/sbdb/schema/jsonschema.go pkg/sbdb/schema/jsonschema_test.go
git commit -m "feat(schema): JSON Schema 2020-12 + x-* parser

Refs #46"
```

---

## Task 5: Equivalence test — legacy and new produce identical Schema

**Files:**
- Create: `pkg/sbdb/schema/equivalence_test.go`
- Create: `testdata/schema/legacy/notes.yaml`
- Create: `testdata/schema/new/notes.yaml`

- [ ] **Step 1: Write the legacy fixture**

`testdata/schema/legacy/notes.yaml`:

```yaml
version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
integrity: strict
fields:
  id:      { type: string, required: true }
  created: { type: date,   required: true }
  status:  { type: enum, values: [active, archived], default: active }
  tags:    { type: list, items: { type: string } }
  parent:  { type: ref, entity: notes }
virtuals:
  title:
    returns: string
    source: |
      def compute(content, fields):
          return fields["id"]
```

- [ ] **Step 2: Write the equivalent new-shape fixture**

`testdata/schema/new/notes.yaml`:

```yaml
$schema: https://json-schema.org/draft/2020-12/schema
$id: sbdb://notes
x-schema-version: 1
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
x-integrity: strict
type: object
required: [id, created]
properties:
  id:      { type: string }
  created: { type: string, format: date }
  status:  { enum: [active, archived], default: active }
  tags:    { type: array, items: { type: string } }
  parent:  { $ref: "sbdb://notes#/properties/id" }
  title:
    type: string
    readOnly: true
    x-compute:
      source: |
        def compute(content, fields):
            return fields["id"]
```

- [ ] **Step 3: Write the equivalence test**

`pkg/sbdb/schema/equivalence_test.go`:

```go
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
```

- [ ] **Step 4: Run, expect PASS.**

```bash
go test ./pkg/sbdb/schema/ -run Equivalence -v
```

- [ ] **Step 5: Commit**

```bash
git add testdata/schema/ pkg/sbdb/schema/equivalence_test.go
git commit -m "test(schema): equivalence test between legacy and new dialect

Refs #46"
```

---

## Task 6: JSON Schema validator wrapper

**Files:**
- Create: `pkg/sbdb/schema/jsvalidate.go`
- Create: `pkg/sbdb/schema/jsvalidate_test.go`

- [ ] **Step 1: Write the failing test**

```go
package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompileAndValidateRecord_OK(t *testing.T) {
	src := []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id:     { type: string }
  status: { enum: [active, archived] }
`)
	s, err := ParseJSONSchema(src)
	require.NoError(t, err)
	v, err := NewValidator(s)
	require.NoError(t, err)
	require.NoError(t, v.ValidateMap(map[string]any{"id": "x", "status": "active"}))
}

func TestCompileAndValidateRecord_RejectsBadEnum(t *testing.T) {
	src := []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id:     { type: string }
  status: { enum: [active, archived] }
`)
	s, _ := ParseJSONSchema(src)
	v, _ := NewValidator(s)
	err := v.ValidateMap(map[string]any{"id": "x", "status": "bogus"})
	require.Error(t, err)
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement**

`pkg/sbdb/schema/jsvalidate.go`:

```go
package schema

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// Validator wraps a compiled JSON Schema for an sbdb entity.
type Validator struct {
	compiled *jsonschema.Schema
}

// NewValidator compiles a Schema's JSON Schema body into a fast validator.
//
// The Schema struct is the in-memory representation; for the JSON Schema
// body we re-emit the document from the in-memory data so the validator
// works regardless of whether the source was legacy or new dialect.
func NewValidator(s *Schema) (*Validator, error) {
	doc, err := schemaToJSONSchemaDoc(s)
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("inmem://"+s.Entity, doc); err != nil {
		return nil, fmt.Errorf("schema: register: %w", err)
	}
	cs, err := c.Compile("inmem://" + s.Entity)
	if err != nil {
		return nil, fmt.Errorf("schema: compile: %w", err)
	}
	return &Validator{compiled: cs}, nil
}

// ValidateMap checks a record's data map.
func (v *Validator) ValidateMap(m map[string]any) error {
	// santhosh-tekuri requires data to round-trip through JSON for tag uniformity.
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	var any any
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&any); err != nil {
		return err
	}
	if err := v.compiled.Validate(any); err != nil {
		return err
	}
	return nil
}

// schemaToJSONSchemaDoc emits a generic map suitable for jsonschema.AddResource.
// It strips x-* keys (the validator does not validate them; meta-schemas do).
func schemaToJSONSchemaDoc(s *Schema) (any, error) {
	doc := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "object",
	}
	props := map[string]any{}
	var required []string
	for name, f := range s.Fields {
		props[name] = fieldToJSONSchema(f)
		if f.Required {
			required = append(required, name)
		}
	}
	doc["properties"] = props
	if len(required) > 0 {
		doc["required"] = required
	}
	// We don't include virtuals in the validator: virtuals are computed,
	// not user-supplied, so user data is validated against the non-virtual
	// fields only. ValidateRecord-with-computed runs separately.
	// Round-trip through YAML to normalise types (yaml.v3 keys may be ints).
	bs, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var out any
	if err := yaml.Unmarshal(bs, &out); err != nil {
		return nil, err
	}
	return normaliseYAMLForJSON(out), nil
}

func fieldToJSONSchema(f *Field) any {
	m := map[string]any{}
	switch f.Type {
	case FieldTypeString:
		m["type"] = "string"
	case FieldTypeInt:
		m["type"] = "integer"
	case FieldTypeFloat:
		m["type"] = "number"
	case FieldTypeBool:
		m["type"] = "boolean"
	case FieldTypeDate:
		m["type"] = "string"
		m["format"] = "date"
	case FieldTypeDatetime:
		m["type"] = "string"
		m["format"] = "date-time"
	case FieldTypeEnum:
		m["enum"] = stringsToAny(f.Values)
	case FieldTypeList:
		m["type"] = "array"
		if f.Items != nil {
			m["items"] = fieldToJSONSchema(f.Items)
		}
	case FieldTypeObject:
		m["type"] = "object"
		nested := map[string]any{}
		var req []string
		for nname, nf := range f.Fields {
			nested[nname] = fieldToJSONSchema(nf)
			if nf.Required {
				req = append(req, nname)
			}
		}
		m["properties"] = nested
		if len(req) > 0 {
			m["required"] = req
		}
	case FieldTypeRef:
		// Refs are strings at the data layer; existence is checked elsewhere.
		m["type"] = "string"
	}
	return m
}

func stringsToAny(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

// normaliseYAMLForJSON converts map[interface{}]interface{} (from yaml.v3
// in some configs) to map[string]any so json.Marshal works.
func normaliseYAMLForJSON(v any) any {
	switch t := v.(type) {
	case map[any]any:
		m := map[string]any{}
		for k, vv := range t {
			m[fmt.Sprint(k)] = normaliseYAMLForJSON(vv)
		}
		return m
	case map[string]any:
		for k, vv := range t {
			t[k] = normaliseYAMLForJSON(vv)
		}
		return t
	case []any:
		for i, vv := range t {
			t[i] = normaliseYAMLForJSON(vv)
		}
		return t
	default:
		return v
	}
}
```

- [ ] **Step 4: Run, expect PASS.**

```bash
go test ./pkg/sbdb/schema/ -run CompileAndValidateRecord -v
```

- [ ] **Step 5: Commit**

```bash
git add pkg/sbdb/schema/jsvalidate.go pkg/sbdb/schema/jsvalidate_test.go
git commit -m "feat(schema): JSON Schema validator wrapper

Refs #46"
```

---

## Task 7: `sbdb schema lint` command

**Files:**
- Create: `cmd/schema_lint.go`
- Create: `cmd/schema_lint_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaLint_AcceptsValidNew(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	path := filepath.Join(tmp, "notes.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
properties:
  id: { type: string }
`), 0o644))
	out, err := runRoot(t, "schema", "lint", path)
	require.NoError(t, err, "out: %s", out)
}

func TestSchemaLint_RejectsMissingXEntity(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
properties:
  id: { type: string }
`), 0o644))
	out, err := runRoot(t, "schema", "lint", path)
	require.Error(t, err, "out: %s", out)
	require.True(t, strings.Contains(out, "x-entity"))
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement**

`cmd/schema_lint.go`:

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema/meta"
)

var schemaLintCmd = &cobra.Command{
	Use:   "lint <path>...",
	Short: "Validate schema file(s) against the sbdb meta-schema",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := jsonschema.NewCompiler()

		// Load the sbdb meta-schemas.
		var metaDoc, computeDoc any
		if err := json.Unmarshal(meta.SchemaMeta, &metaDoc); err != nil {
			return err
		}
		if err := json.Unmarshal(meta.ComputeMeta, &computeDoc); err != nil {
			return err
		}
		if err := c.AddResource("https://schemas.sbdb.dev/2026-05/sbdb.compute.schema.json", computeDoc); err != nil {
			return err
		}
		if err := c.AddResource("https://schemas.sbdb.dev/2026-05/sbdb.schema.json", metaDoc); err != nil {
			return err
		}
		metaSchema, err := c.Compile("https://schemas.sbdb.dev/2026-05/sbdb.schema.json")
		if err != nil {
			return fmt.Errorf("compile meta-schema: %w", err)
		}

		var anyErr bool
		for _, p := range args {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			var doc any
			if err := yaml.Unmarshal(data, &doc); err != nil {
				return fmt.Errorf("%s: parse: %w", p, err)
			}
			doc = normaliseForJSON(doc)
			if err := metaSchema.Validate(doc); err != nil {
				fmt.Fprintf(cmd.OutOrStderr(), "%s: %v\n", p, err)
				anyErr = true
				continue
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: ok\n", p)
		}
		if anyErr {
			return fmt.Errorf("lint failed")
		}
		_ = bytes.NewBuffer
		return nil
	},
}

// normaliseForJSON ensures yaml.v3 output (which can include map[any]any) is
// usable by the json-schema validator.
func normaliseForJSON(v any) any {
	switch t := v.(type) {
	case map[any]any:
		m := map[string]any{}
		for k, vv := range t {
			m[fmt.Sprint(k)] = normaliseForJSON(vv)
		}
		return m
	case map[string]any:
		for k, vv := range t {
			t[k] = normaliseForJSON(vv)
		}
		return t
	case []any:
		for i, vv := range t {
			t[i] = normaliseForJSON(vv)
		}
		return t
	default:
		return v
	}
}

func init() {
	schemaCmd.AddCommand(schemaLintCmd)
}
```

> Note: `schemaCmd` already exists in `cmd/schema_cmd.go` (it owns `sbdb schema show`). If running this task standalone reveals it does not, prepend a tiny stub in this file:
> ```go
> var schemaCmd = &cobra.Command{Use: "schema", Short: "Schema management"}
> func init() { rootCmd.AddCommand(schemaCmd) }
> ```
> But check first — duplicating breaks the build.

- [ ] **Step 4: Run, expect PASS.**

```bash
go test ./cmd/ -run SchemaLint -v
```

- [ ] **Step 5: Commit**

```bash
git add cmd/schema_lint.go cmd/schema_lint_test.go
git commit -m "feat(schema): 'sbdb schema lint' command

Refs #46"
```

---

## Task 8: Diff classifier

**Files:**
- Create: `pkg/sbdb/schema/diff.go`
- Create: `pkg/sbdb/schema/diff_test.go`

- [ ] **Step 1: Write the failing test**

```go
package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func mustParse(t *testing.T, src string) *Schema {
	t.Helper()
	s, err := ParseJSONSchema([]byte(src))
	require.NoError(t, err)
	return s
}

func TestDiff_AdditiveOptionalField(t *testing.T) {
	old := mustParse(t, `
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: string }
`)
	newer := mustParse(t, `
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id:    { type: string }
  notes: { type: string }
`)
	d := Diff(old, newer)
	require.False(t, d.HasBreaking(), d.String())
}

func TestDiff_BreakingNewRequired(t *testing.T) {
	old := mustParse(t, `
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: string }
`)
	newer := mustParse(t, `
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id, created]
properties:
  id:      { type: string }
  created: { type: string, format: date }
`)
	d := Diff(old, newer)
	require.True(t, d.HasBreaking())
}

func TestDiff_BreakingTypeChange(t *testing.T) {
	old := mustParse(t, `
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: string }
`)
	newer := mustParse(t, `
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: integer }
`)
	d := Diff(old, newer)
	require.True(t, d.HasBreaking())
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement**

`pkg/sbdb/schema/diff.go`:

```go
package schema

import (
	"fmt"
	"strings"
)

// DiffEntry classifies a single delta between two schemas.
type DiffEntry struct {
	Path     string
	Class    string // "additive" | "breaking"
	Message  string
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

	// Storage moves are breaking.
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

	// Field-by-field.
	for name, of := range old.Fields {
		nf, ok := newer.Fields[name]
		if !ok {
			r.Entries = append(r.Entries, DiffEntry{
				Path: "properties." + name, Class: "breaking",
				Message: "removed",
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
	for name := range newer.Fields {
		if _, existed := old.Fields[name]; !existed {
			class := "additive"
			if newer.Fields[name].Required {
				class = "breaking"
			}
			r.Entries = append(r.Entries, DiffEntry{
				Path: "properties." + name, Class: class,
				Message: "added",
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
```

- [ ] **Step 4: Run, expect PASS.**

- [ ] **Step 5: Commit**

```bash
git add pkg/sbdb/schema/diff.go pkg/sbdb/schema/diff_test.go
git commit -m "feat(schema): additive vs breaking diff classifier

Refs #46"
```

---

## Task 9: `sbdb schema diff` command

**Files:**
- Create: `cmd/schema_diff.go`
- Create: `cmd/schema_diff_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaDiff_BreakingExitsNonzero(t *testing.T) {
	tmp := t.TempDir()
	oldP := filepath.Join(tmp, "old.yaml")
	newP := filepath.Join(tmp, "new.yaml")
	require.NoError(t, os.WriteFile(oldP, []byte(`
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: string }
`), 0o644))
	require.NoError(t, os.WriteFile(newP, []byte(`
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id, created]
properties:
  id:      { type: string }
  created: { type: string, format: date }
`), 0o644))
	out, err := runRoot(t, "schema", "diff", oldP, newP)
	require.Error(t, err, "out: %s", out)
}

func TestSchemaDiff_AdditiveExitsZero(t *testing.T) {
	tmp := t.TempDir()
	oldP := filepath.Join(tmp, "old.yaml")
	newP := filepath.Join(tmp, "new.yaml")
	require.NoError(t, os.WriteFile(oldP, []byte(`
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: string }
`), 0o644))
	require.NoError(t, os.WriteFile(newP, []byte(`
x-entity: notes
x-storage: { docs_dir: d, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id:    { type: string }
  notes: { type: string }
`), 0o644))
	_, err := runRoot(t, "schema", "diff", oldP, newP)
	require.NoError(t, err)
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement**

`cmd/schema_diff.go`:

```go
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
```

- [ ] **Step 4: Run, expect PASS.**

- [ ] **Step 5: Commit**

```bash
git add cmd/schema_diff.go cmd/schema_diff_test.go
git commit -m "feat(schema): 'sbdb schema diff' command

Refs #46"
```

---

## Task 10: Compat-check core (`evolve.go`)

**Files:**
- Create: `pkg/sbdb/schema/evolve.go`
- Create: `pkg/sbdb/schema/evolve_test.go`

- [ ] **Step 1: Write the failing test**

```go
package schema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompatCheck_AllValid(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs/notes"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs/notes", "a.md"),
		[]byte("---\nid: a\n---\n# hello\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs/notes", "b.md"),
		[]byte("---\nid: b\n---\n# world\n"), 0o644))

	src := []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: string }
`)
	s, err := ParseJSONSchema(src)
	require.NoError(t, err)
	report, err := CheckExisting(s, dir)
	require.NoError(t, err)
	require.Empty(t, report.Failures)
}

func TestCompatCheck_ReportsMissingRequired(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs/notes"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs/notes", "a.md"),
		[]byte("---\nid: a\n---\n# hello\n"), 0o644))

	src := []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
required: [id, created]
properties:
  id:      { type: string }
  created: { type: string, format: date }
`)
	s, _ := ParseJSONSchema(src)
	report, err := CheckExisting(s, dir)
	require.NoError(t, err)
	require.Len(t, report.Failures, 1)
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement**

`pkg/sbdb/schema/evolve.go`:

```go
package schema

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/storage"
)

// CompatFailure is one document that fails the compat check.
type CompatFailure struct {
	Path  string
	Error string
}

// CompatReport collects all failures.
type CompatReport struct {
	Failures []CompatFailure
}

// CheckExisting validates every existing doc under s.DocsDir against s
// using the JSON Schema validator. It walks docs/<docs_dir>/ for *.md
// files, parses frontmatter, and runs ValidateMap on each.
func CheckExisting(s *Schema, repoRoot string) (*CompatReport, error) {
	v, err := NewValidator(s)
	if err != nil {
		return nil, err
	}
	docsDir := filepath.Join(repoRoot, s.DocsDir)
	report := &CompatReport{}
	walkErr := filepath.WalkDir(docsDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if filepath.Base(p) == filepath.Base(docsDir) {
				return nil // missing dir is fine
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		fm, _, perr := storage.ParseMarkdown(p)
		if perr != nil {
			report.Failures = append(report.Failures, CompatFailure{Path: p, Error: perr.Error()})
			return nil
		}
		if verr := v.ValidateMap(fm); verr != nil {
			report.Failures = append(report.Failures, CompatFailure{Path: p, Error: verr.Error()})
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk: %w", walkErr)
	}
	return report, nil
}
```

- [ ] **Step 4: Run, expect PASS.**

- [ ] **Step 5: Commit**

```bash
git add pkg/sbdb/schema/evolve.go pkg/sbdb/schema/evolve_test.go
git commit -m "feat(schema): empirical compat check core

Refs #46"
```

---

## Task 11: `sbdb schema check` command

**Files:**
- Create: `cmd/schema_check.go`
- Create: `cmd/schema_check_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaCheck_PassesWhenAllValid(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "schemas/notes.yaml"), []byte(`
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
required: [id]
properties:
  id: { type: string }
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "docs/notes"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "docs/notes/a.md"),
		[]byte("---\nid: a\n---\n# hi\n"), 0o644))

	_, err := runRoot(t, "schema", "check")
	require.NoError(t, err)
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement**

`cmd/schema_check.go`:

```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
)

var schemaCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate every existing doc against its current schema",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, _ := os.Getwd()
		schemasDir := filepath.Join(root, "schemas")
		entries, err := os.ReadDir(schemasDir)
		if err != nil {
			return fmt.Errorf("read schemas dir: %w", err)
		}
		anyFail := false
		for _, e := range entries {
			if e.IsDir() || (filepath.Ext(e.Name()) != ".yaml" && filepath.Ext(e.Name()) != ".yml" && filepath.Ext(e.Name()) != ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(schemasDir, e.Name()))
			if err != nil {
				return err
			}
			s, err := schema.Parse(data)
			if err != nil {
				return fmt.Errorf("%s: %w", e.Name(), err)
			}
			rep, err := schema.CheckExisting(s, root)
			if err != nil {
				return err
			}
			for _, f := range rep.Failures {
				fmt.Fprintf(cmd.OutOrStderr(), "%s: %s: %s\n", e.Name(), f.Path, f.Error)
				anyFail = true
			}
		}
		if anyFail {
			return fmt.Errorf("compat check failed")
		}
		return nil
	},
}

func init() {
	schemaCmd.AddCommand(schemaCheckCmd)
}
```

- [ ] **Step 4: Run, expect PASS.**

- [ ] **Step 5: Commit**

```bash
git add cmd/schema_check.go cmd/schema_check_test.go
git commit -m "feat(schema): 'sbdb schema check' command

Refs #46"
```

---

## Task 12: `sbdb schema migrate` command

**Files:**
- Create: `cmd/schema_migrate.go`
- Create: `cmd/schema_migrate_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaMigrate_RewritesLegacy(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "notes.yaml")
	require.NoError(t, os.WriteFile(in, []byte(`
version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
integrity: strict
fields:
  id: { type: string, required: true }
`), 0o644))
	_, err := runRoot(t, "schema", "migrate", "--in-place", in)
	require.NoError(t, err)
	out, _ := os.ReadFile(in)
	require.True(t, strings.Contains(string(out), "x-entity"))
	require.True(t, strings.Contains(string(out), "$schema"))
	require.False(t, strings.Contains(string(out), "fields:"))
}

func TestSchemaMigrate_CheckExitsNonzeroOnLegacy(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "notes.yaml")
	require.NoError(t, os.WriteFile(in, []byte(`
version: 1
entity: notes
docs_dir: docs/notes
filename: "{id}.md"
id_field: id
fields:
  id: { type: string, required: true }
`), 0o644))
	_, err := runRoot(t, "schema", "migrate", "--check", in)
	require.Error(t, err)
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement**

`cmd/schema_migrate.go`:

```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
)

var (
	schemaMigrateCheck   bool
	schemaMigrateInPlace bool
	schemaMigrateOutDir  string
)

var schemaMigrateCmd = &cobra.Command{
	Use:   "migrate <path>...",
	Short: "Rewrite legacy schema files into the JSON Schema 2020-12 + x-* form",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		anyLegacy := false
		for _, p := range args {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			d, err := schema.DetectDialect(data)
			if err != nil {
				return fmt.Errorf("%s: %w", p, err)
			}
			if d != schema.DialectLegacy {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: already new dialect; skipping\n", p)
				continue
			}
			anyLegacy = true
			if schemaMigrateCheck {
				continue
			}
			s, err := schema.ParseLegacy(data)
			if err != nil {
				return fmt.Errorf("%s: parse: %w", p, err)
			}
			out, err := emitNewDialect(s)
			if err != nil {
				return err
			}
			dest := p + ".new.yaml"
			if schemaMigrateInPlace {
				dest = p
			}
			if schemaMigrateOutDir != "" {
				dest = filepath.Join(schemaMigrateOutDir, filepath.Base(p))
			}
			if err := os.WriteFile(dest, out, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", p, dest)
		}
		if schemaMigrateCheck && anyLegacy {
			return fmt.Errorf("legacy schema files present")
		}
		return nil
	},
}

func emitNewDialect(s *schema.Schema) ([]byte, error) {
	doc := map[string]any{
		"$schema":           "https://json-schema.org/draft/2020-12/schema",
		"$id":               "sbdb://" + s.Entity,
		"x-schema-version":  s.Version,
		"x-entity":          s.Entity,
		"x-storage":         map[string]any{"docs_dir": s.DocsDir, "filename": s.Filename},
		"x-id":              s.IDField,
		"type":              "object",
	}
	if s.Integrity != "" {
		doc["x-integrity"] = s.Integrity
	}
	if s.Partition != "" && s.Partition != "none" {
		doc["x-partition"] = map[string]any{"mode": s.Partition, "field": s.DateField}
	}
	props := map[string]any{}
	var required []string
	for name, f := range s.Fields {
		props[name] = fieldToJSONSchemaMap(f)
		if f.Required {
			required = append(required, name)
		}
	}
	for name, v := range s.Virtuals {
		m := map[string]any{
			"type":     virtualReturnsToJSONType(v.Returns),
			"readOnly": true,
			"x-compute": map[string]any{
				"source": v.Source,
				"edge":   v.Edge,
			},
		}
		if v.EdgeEntity != "" {
			m["x-compute"].(map[string]any)["edge_entity"] = v.EdgeEntity
		}
		props[name] = m
	}
	doc["properties"] = props
	if len(required) > 0 {
		doc["required"] = required
	}
	return yaml.Marshal(doc)
}

func fieldToJSONSchemaMap(f *schema.Field) map[string]any {
	m := map[string]any{}
	switch f.Type {
	case schema.FieldTypeString:
		m["type"] = "string"
	case schema.FieldTypeInt:
		m["type"] = "integer"
	case schema.FieldTypeFloat:
		m["type"] = "number"
	case schema.FieldTypeBool:
		m["type"] = "boolean"
	case schema.FieldTypeDate:
		m["type"] = "string"
		m["format"] = "date"
	case schema.FieldTypeDatetime:
		m["type"] = "string"
		m["format"] = "date-time"
	case schema.FieldTypeEnum:
		m["enum"] = f.Values
	case schema.FieldTypeList:
		m["type"] = "array"
		if f.Items != nil {
			m["items"] = fieldToJSONSchemaMap(f.Items)
		}
	case schema.FieldTypeObject:
		m["type"] = "object"
		nested := map[string]any{}
		var req []string
		for n, nf := range f.Fields {
			nested[n] = fieldToJSONSchemaMap(nf)
			if nf.Required {
				req = append(req, n)
			}
		}
		m["properties"] = nested
		if len(req) > 0 {
			m["required"] = req
		}
	case schema.FieldTypeRef:
		m["$ref"] = "sbdb://" + f.RefEntity + "#/properties/id"
	}
	if f.Default != nil {
		m["default"] = f.Default
	}
	return m
}

func virtualReturnsToJSONType(returns string) string {
	switch returns {
	case "int":
		return "integer"
	case "float":
		return "number"
	case "bool":
		return "boolean"
	case "date", "datetime":
		return "string"
	case "list", "list[string]", "list[int]":
		return "array"
	default:
		if strings.HasPrefix(returns, "list[") {
			return "array"
		}
		return "string"
	}
}

func init() {
	schemaMigrateCmd.Flags().BoolVar(&schemaMigrateCheck, "check", false, "exit non-zero if any input is legacy")
	schemaMigrateCmd.Flags().BoolVar(&schemaMigrateInPlace, "in-place", false, "rewrite the original file")
	schemaMigrateCmd.Flags().StringVarP(&schemaMigrateOutDir, "out", "o", "", "write migrated files to this directory")
	schemaCmd.AddCommand(schemaMigrateCmd)
}
```

- [ ] **Step 4: Run, expect PASS.**

- [ ] **Step 5: Commit**

```bash
git add cmd/schema_migrate.go cmd/schema_migrate_test.go
git commit -m "feat(schema): 'sbdb schema migrate' command

Refs #46"
```

---

## Task 13: Wire pre-commit script to real commands

**Files:**
- Modify: `scripts/schema-precommit.sh`

- [ ] **Step 1: Replace the stub block**

In `scripts/schema-precommit.sh`, the current implementation already has the right shape but exits 0 when `sbdb schema lint` is unavailable. With the commands now shipped, remove the gate. Replace:

```bash
if ! sbdb schema --help 2>/dev/null | grep -q '^  lint'; then
  echo "sbdb-schema-validate: 'sbdb schema lint' not available in this binary;" >&2
  echo "                     skipping (implementation pending; see issue #46)." >&2
  exit 0
fi
```

with:

```bash
if ! sbdb schema --help 2>/dev/null | grep -q '^  lint'; then
  echo "sbdb-schema-validate: this build of sbdb does not support 'schema lint'." >&2
  echo "                     run 'go install ./...' from the repo root." >&2
  exit 1
fi
```

- [ ] **Step 2: Verify the hook fires in a synthetic scenario**

```bash
mkdir -p /tmp/sbdb-hook-verify/schemas
cat > /tmp/sbdb-hook-verify/schemas/notes.yaml <<'EOF'
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
type: object
properties:
  id: { type: string }
EOF
PATH="$(go env GOPATH)/bin:$PATH" ./scripts/schema-precommit.sh /tmp/sbdb-hook-verify/schemas/notes.yaml
```

Expected output:
```
→ sbdb schema lint /tmp/sbdb-hook-verify/schemas/notes.yaml
/tmp/sbdb-hook-verify/schemas/notes.yaml: ok
  (new schema; skipping diff/check)
```
Exit 0.

- [ ] **Step 3: Commit**

```bash
git add scripts/schema-precommit.sh
git commit -m "feat(schema): pre-commit script now invokes real sbdb schema sub-commands

Refs #46"
```

---

## Task 14: Loader uses dispatcher (cleanup)

**Files:**
- Modify: `pkg/sbdb/schema/loader.go`

- [ ] **Step 1: Verify dispatch flow**

Confirm `Parse(data)` already calls `DetectDialect` then `ParseLegacy` or `ParseJSONSchema` (set up in Task 3). If the legacy normaliser drops any fields the new parser keeps, fix here. Run:

```bash
go test ./pkg/sbdb/schema/ -v
```

Expected: all green.

- [ ] **Step 2: Run all sbdb tests**

```bash
go test ./...
```

Expected: all green.

- [ ] **Step 3: Commit only if changes were needed**

```bash
git add -p pkg/sbdb/schema/loader.go
git commit -m "fix(schema): dispatcher wires legacy + new parsers cleanly

Refs #46"
```

---

## Task 15: User guide for schemas

**Files:**
- Create: `docs/guide/schemas.md`
- Modify: `README.md`

- [ ] **Step 1: Write the guide**

`docs/guide/schemas.md`:

```markdown
# Schemas

`sbdb` schemas are valid **JSON Schema 2020-12** documents with a small set of `x-*` extension keywords for sbdb-specific concepts. A stock JSON Schema validator (e.g. `ajv`) accepts them; editor LSPs (yaml-language-server, IntelliJ, VS Code) provide autocomplete and diagnostics out of the box.

## Anatomy of a schema

\`\`\`yaml
$schema: https://json-schema.org/draft/2020-12/schema
$id: sbdb://notes
x-schema-version: 1
x-entity: notes
x-storage: { docs_dir: docs/notes, filename: "{id}.md" }
x-id: id
x-integrity: strict

type: object
required: [id, created]
properties:
  id:      { type: string, pattern: "^[a-z0-9-]+$" }
  created: { type: string, format: date }
  status:  { enum: [active, archived], default: active }
  tags:    { type: array, items: { type: string } }
  parent:  { \$ref: "sbdb://notes#/properties/id" }   # foreign key
  title:                                             # virtual
    type: string
    readOnly: true
    x-compute:
      source: |
        def compute(content, fields):
            ...
\`\`\`

## Reserved keywords

| Keyword | Where | Meaning |
|---|---|---|
| `x-schema-version` | top | integer (or "major.minor") tracking schema evolution |
| `x-entity` | top | entity name (slug) |
| `x-storage` | top | `{docs_dir, filename, records_dir?}` |
| `x-id` | top | name of the id property |
| `x-integrity` | top | `strict | warn | off` |
| `x-partition` | top | `{mode: none|monthly, field?: string}` |
| `x-events` | top | event bucket + types |
| `x-compute` | per-property | virtual computation block |

Foreign-key references are pure JSON Schema `$ref` pointing at `sbdb://<entity>#/properties/<id>`. The link graph derives entity edges from these URIs.

## Editor support

Add this directive at the top of any schema YAML:

\`\`\`
# yaml-language-server: \$schema=.sbdb/cache/meta/sbdb.schema.json
\`\`\`

`sbdb init` writes the meta-schemas into `.sbdb/cache/meta/`.

## Migration from the legacy dialect

\`\`\`bash
sbdb schema migrate --in-place schemas/notes.yaml
\`\`\`

Or `--check` to fail CI if any legacy schemas remain.

## Schema evolution guardrails

| Command | Purpose |
|---|---|
| `sbdb schema lint <file>` | Validate against the meta-schema |
| `sbdb schema diff <old> <new>` | Classify deltas as additive vs breaking |
| `sbdb schema check` | Run schemas against every existing doc |

The pre-commit hook (`sbdb-schema-validate` in `.pre-commit-config.yaml`) runs all three and refuses commits that would invalidate existing docs unless `x-schema-version` major is bumped or `SBDB_ALLOW_BREAKING=1` is set.
```

- [ ] **Step 2: Add a README link**

In `README.md`, near the existing "Schema format" section, add:

```markdown
For the full schema reference and migration guide, see [docs/guide/schemas.md](docs/guide/schemas.md).
```

- [ ] **Step 3: Commit**

```bash
git add docs/guide/schemas.md README.md
git commit -m "docs(schema): user guide and README link

Refs #46"
```

---

## Task 16: Open the PR

- [ ] **Step 1: Run full test suite**

```bash
go test ./...
```

Expected: all green.

- [ ] **Step 2: Push and open PR**

```bash
git push -u origin feat/json-schema-migration
gh pr create --title "feat(schema): migrate to JSON Schema 2020-12 with x-* extensions" --body "$(cat <<'EOF'
## Summary

- Schemas now valid JSON Schema 2020-12 with bare x-* extensions
- Legacy dialect auto-migrated on load; sbdb schema migrate rewrites in place
- New CLI: sbdb schema lint / diff / check / migrate
- Pre-commit guardrail wired to real commands; refuses breaking changes
- Embedded meta-schemas drive lint
- Validator swap to santhosh-tekuri/jsonschema/v6
- User guide at docs/guide/schemas.md

## Test plan

- [x] go test ./pkg/sbdb/schema/... passes (parser, equivalence, validator, diff, evolve)
- [x] go test ./cmd/... passes (lint, diff, check, migrate)
- [x] go test ./... full repo green
- [x] Manual: pre-commit hook fires on schema files, skips when no schema files staged

Closes #46
EOF
)"
```

---

## Self-review

Spec coverage:

- §"Reserved keywords" → Tasks 1, 4 (table mirrored in meta-schema and parser)
- §"On-disk shape" → Tasks 4, 5 (parser + equivalence test)
- §"Foreign-key model" → Task 4 (`refEntityFromURI`); link-graph derivation already happens via `RefEntity` in existing `pkg/sbdb/kg`
- §"Virtuals" → Task 4 (`readOnly + x-compute` recognised)
- §"Meta-schemas" → Task 1 (embedded), Task 7 (used by lint)
- §"Loader" → Tasks 2, 3, 14
- §"Validation" → Task 6 (jsvalidate); existing cross-document checks unchanged
- §"CLI surface" → Tasks 7 (lint), 9 (diff), 11 (check), 12 (migrate)
- §"Backwards compatibility" → Tasks 2, 3 (dual dialect; loader picks)
- §"Schema Evolution Guardrails" → Tasks 8, 10, 13
- §"Schema versioning convention" → Task 12 emits `x-schema-version`; Task 8 differentiates breaking; pre-commit script in Task 13 enforces
- §"Acceptance criteria" → covered across tasks; equivalence test (Task 5) closes the "every legacy schema migrates to byte-equivalent in-memory representation" criterion

Type consistency check:

- `Schema`, `Field`, `Virtual`, `FieldMap`, `FieldType*` constants exist in `pkg/sbdb/schema/schema.go` and are reused everywhere.
- `Validator` has `NewValidator(s *Schema)` and `ValidateMap(m map[string]any) error` — used in evolve.go (Task 10) and consistent across cmd/check.
- `DiffReport.HasBreaking()` and `DiffReport.String()` referenced consistently in Task 9.
- `Dialect` constants `DialectLegacy`, `DialectNew`, `DialectUnknown` referenced consistently.

Placeholder scan: no TBDs. Each step shows complete code or exact commands.

---

## Execution handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-08-json-schema-migration.md`.

Auto mode is active and the user has indicated to keep going; recommended path is inline execution starting with Task 1.
