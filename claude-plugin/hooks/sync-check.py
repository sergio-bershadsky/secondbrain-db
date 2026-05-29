#!/usr/bin/env python3
"""Stop hook: surface external-sync drift at end of turn.

Runs `sbdb sync check --format json` and, if any doc is out of sync with its
declared external target (Confluence / Jira / Notion / Slack), surfaces a
one-line summary to the transcript. Claude can then offer to push via the
`secondbrain-db-sync` skill.

This hook NEVER blocks and NEVER pushes. `sbdb sync check` is local-only (no
network, no credentials). Its exit code 4 (drift present) is expected and is
not treated as a failure here. The user remains in control of every push.

Stays silent when there are no integrations configured or nothing has
drifted, so clean turns add no noise.
"""

import json
import os
import subprocess
import sys


def main():
    project_root = find_project_root(os.getcwd())
    if not project_root:
        return  # not an sbdb project

    # No integrations configured → nothing to check, stay silent.
    if not os.path.isdir(os.path.join(project_root, ".sbdb", "integrations")):
        return

    sbdb = find_sbdb()
    if not sbdb:
        return  # no CLI installed; nothing we can do

    try:
        result = subprocess.run(
            [sbdb, "sync", "check", "--format", "json", "-b", project_root],
            capture_output=True,
            text=True,
            timeout=15,
        )
    except (subprocess.TimeoutExpired, FileNotFoundError):
        return

    summary = format_summary(result.stdout)
    if summary:
        # Surface a single line to the transcript. We never set `decision`,
        # so this is informational only and never blocks the Stop event.
        print(json.dumps({"message": summary}))


def format_summary(stdout):
    """Return a one-line drift summary, or empty string when all in sync."""
    try:
        report = json.loads(stdout)
    except (json.JSONDecodeError, TypeError):
        return ""
    if not isinstance(report, dict):
        return ""

    summary = report.get("summary") or {}
    # summary is a map of result-name -> count. Anything other than in_sync
    # is worth surfacing.
    drift_counts = {k: v for k, v in summary.items() if k != "in_sync" and v}
    if not drift_counts:
        return ""

    # Build a compact, deterministic phrase, e.g.:
    # "[sbdb] external sync: 2 local_drift, 1 never_published — run /sbdb or ask to push"
    ordered = sorted(drift_counts.items())
    parts = [f"{count} {name}" for name, count in ordered]
    return (
        f"[sbdb] external sync: {', '.join(parts)}"
        " — ask to push (uses the secondbrain-db-sync skill)"
    )


def find_project_root(start_dir):
    directory = os.path.abspath(start_dir)
    for _ in range(10):
        if os.path.exists(os.path.join(directory, ".sbdb.toml")):
            return directory
        parent = os.path.dirname(directory)
        if parent == directory:
            break
        directory = parent
    return None


def find_sbdb():
    for path_dir in os.environ.get("PATH", "").split(os.pathsep):
        candidate = os.path.join(path_dir, "sbdb")
        if os.path.isfile(candidate) and os.access(candidate, os.X_OK):
            return candidate
    home = os.path.expanduser("~")
    for candidate in [os.path.join(home, "go", "bin", "sbdb"), "/usr/local/bin/sbdb"]:
        if os.path.isfile(candidate) and os.access(candidate, os.X_OK):
            return candidate
    return None


if __name__ == "__main__":
    # Drain stdin so the harness doesn't block waiting for us.
    try:
        sys.stdin.read()
    except Exception:
        pass
    main()
