# sbdb doctor GitHub Action

Reusable GitHub Action that runs `sbdb doctor check` on your knowledge base repository. Detects drift (frontmatter vs records mismatch) and tamper (files edited outside sbdb).

## Usage

Add this to your secondbrain repository's `.github/workflows/doctor.yml`:

```yaml
name: KB Health Check

on:
  push:
    branches: [main]
    paths:
      - "docs/**"
      - "data/**"
      - "schemas/**"
  pull_request:
    paths:
      - "docs/**"
      - "data/**"
      - "schemas/**"

jobs:
  doctor:
    name: Doctor Check
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.23"
          cache: true
      - uses: sergio-bershadsky/secondbrain-db/.github/actions/doctor@main
        with:
          schema: "notes"
```

## Inputs

| Input | Description | Default |
|-------|-------------|---------|
| `schema` | Schema name to check. Empty = check all. | `""` |
| `base-path` | Project root directory | `.` |
| `sbdb-version` | sbdb version to install | `latest` |
| `fail-on-drift` | Fail if drift detected | `true` |
| `fail-on-tamper` | Fail if tamper detected | `true` |
| `check-untracked` | Verify untracked files too | `true` |

## Outputs

| Output | Description |
|--------|-------------|
| `drift-count` | Number of drift issues found |
| `tamper-count` | Number of tamper issues found |
| `status` | `clean`, `drift`, `tamper`, or `both` |

## Examples

### Check all schemas

```yaml
- uses: sergio-bershadsky/secondbrain-db/.github/actions/doctor@main
```

### Check specific schema, warn on drift but fail on tamper

```yaml
- uses: sergio-bershadsky/secondbrain-db/.github/actions/doctor@main
  with:
    schema: "decisions"
    fail-on-drift: "false"
    fail-on-tamper: "true"
```

### Use output in subsequent steps

```yaml
- uses: sergio-bershadsky/secondbrain-db/.github/actions/doctor@main
  id: doctor
- run: echo "Status: ${{ steps.doctor.outputs.status }}"
```
