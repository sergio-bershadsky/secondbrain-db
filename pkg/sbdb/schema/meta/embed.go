// Package meta holds the embedded meta-schemas for the sbdb dialect of
// JSON Schema 2020-12.
package meta

import _ "embed"

//go:embed sbdb.schema.json
var SchemaMeta []byte

//go:embed sbdb.compute.schema.json
var ComputeMeta []byte
