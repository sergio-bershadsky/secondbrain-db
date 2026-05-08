#!/usr/bin/env bash
# Pre-commit guardrail for sbdb schemas.
#
# For each staged schema file, runs:
#   1. sbdb schema lint  — meta-schema validation
#   2. sbdb schema diff HEAD:<file> <file>  — classify additive vs breaking
#   3. If breaking, sbdb schema check --against HEAD  — verify every existing
#      doc still validates and that x-schema-version major has been bumped.
#
# Override the breaking-check (only the breaking-check; lint and diff still run)
# by exporting SBDB_ALLOW_BREAKING=1.
#
# This script is wired up by .pre-commit-config.yaml. It is invoked with the
# list of staged schema files as positional arguments.
#
# Until the schema sub-commands ship (issue #46), this script is a stub that
# only verifies that the sbdb binary is on PATH for the commands that will
# eventually run. Once `sbdb schema lint|diff|check` exist, replace the stub
# block below with the real invocations.

set -euo pipefail

if [ "$#" -eq 0 ]; then
  exit 0
fi

if ! command -v sbdb >/dev/null 2>&1; then
  echo "sbdb-schema-validate: 'sbdb' binary not on PATH; skipping schema checks." >&2
  echo "                     install with 'go install ./...' from the repo root." >&2
  exit 0
fi

# Detect whether the sbdb binary already speaks the new sub-commands. If not,
# this is a pre-implementation commit; warn but do not fail so work can proceed
# on the implementation branch itself.
if ! sbdb schema --help 2>/dev/null | grep -q '^  lint'; then
  echo "sbdb-schema-validate: 'sbdb schema lint' not available in this binary;" >&2
  echo "                     skipping (implementation pending; see issue #46)." >&2
  exit 0
fi

fail=0
for schema in "$@"; do
  echo "→ sbdb schema lint $schema"
  if ! sbdb schema lint "$schema"; then
    fail=1
    continue
  fi

  # Skip diff/check for newly added schemas: nothing to diff against, no
  # existing docs guaranteed to match.
  if ! git cat-file -e "HEAD:$schema" 2>/dev/null; then
    echo "  (new schema; skipping diff/check)"
    continue
  fi

  echo "→ sbdb schema diff HEAD:$schema $schema"
  diff_exit=0
  sbdb schema diff "HEAD:$schema" "$schema" || diff_exit=$?

  if [ "$diff_exit" -ne 0 ]; then
    if [ "${SBDB_ALLOW_BREAKING:-0}" = "1" ]; then
      echo "  SBDB_ALLOW_BREAKING=1 set; skipping doc compatibility check." >&2
      continue
    fi
    echo "→ sbdb schema check --against HEAD"
    if ! sbdb schema check --against HEAD; then
      echo "  ✗ existing docs would fail under the new schema." >&2
      echo "    Migrate the docs in this same commit, or set" >&2
      echo "    SBDB_ALLOW_BREAKING=1 to override (only do this if you know why)." >&2
      fail=1
    fi
  fi
done

exit "$fail"
