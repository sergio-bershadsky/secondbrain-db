#!/usr/bin/env python3
"""Stop / SubagentStop hook: reconcile sidecars in post-fix mode.

When `[claude] mode` is `post-fix` (the default in .sbdb.toml), the agent
is allowed to Edit/Write under docs/ directly. This hook fires at end of
turn and runs `sbdb doctor heal --since HEAD --i-meant-it`, which:

  1. Recomputes virtuals from the on-disk markdown.
  2. Re-signs sidecars in lockstep with the new content.

The opt-in to post-fix is itself the human acknowledgement, so the hook
passes --i-meant-it on the user's behalf. In `block` mode this hook is
a no-op (the existing integrity-final-check.py handles that branch).

This hook never blocks. If heal exits non-zero, we surface its stderr
to the transcript and let pre-commit catch anything that slipped
through. Blocking would defeat the whole point of post-fix mode.
"""

import json
import os
import subprocess
import sys


def main():
    project_root = find_project_root(os.getcwd())
    if not project_root:
        return  # not an sbdb project
    if read_claude_mode(project_root) != "post-fix":
        return  # block mode handled by integrity-final-check.py

    sbdb = find_sbdb()
    if not sbdb:
        # No CLI installed — nothing we can do; pre-commit will catch
        # drift if the user commits without first installing sbdb.
        return

    try:
        result = subprocess.run(
            [
                sbdb, "doctor", "heal",
                "--since", "HEAD",
                "--i-meant-it",
                "--format", "json",
                "-b", project_root,
            ],
            capture_output=True,
            text=True,
            timeout=20,
        )
    except (subprocess.TimeoutExpired, FileNotFoundError):
        return

    summary = format_summary(result)
    if not summary:
        return

    # Surface a single line to the transcript via the hook output channel.
    # decision is omitted — we never block.
    print(json.dumps({"message": summary}))


def format_summary(result):
    """Return a one-line summary, or empty string for no-op runs."""
    payload = parse_data(result.stdout)
    if not payload:
        if result.returncode != 0 and result.stderr:
            # Heal failed before producing JSON — surface a brief notice.
            first_line = result.stderr.splitlines()[0] if result.stderr.splitlines() else "heal failed"
            return f"[sbdb] post-fix heal: {first_line}"
        return ""

    counts = payload.get("counts") or {}
    re_signed = counts.get("re_signed", 0)
    drift_fixed = counts.get("drift_fixed", 0)
    skipped = counts.get("skipped_no_schema", 0)
    no_change = counts.get("no_change", 0)
    errors = counts.get("error", 0)

    if not (re_signed or drift_fixed or errors):
        # Nothing changed and no errors — stay silent so we don't add
        # noise to clean turns. The hook fires every Stop event.
        return ""

    parts = []
    if re_signed:
        parts.append(f"{re_signed} re-signed")
    if drift_fixed:
        parts.append(f"{drift_fixed} sidecar(s) recreated")
    if no_change:
        parts.append(f"{no_change} unchanged")
    if skipped:
        parts.append(f"{skipped} skipped (outside any schema)")
    if errors:
        parts.append(f"{errors} error(s) — run `sbdb doctor check --all`")

    return f"[sbdb] post-fix heal: {', '.join(parts)}"


def parse_data(stdout):
    """Extract the inner `data` object from sbdb's `{version, data}` envelope."""
    try:
        envelope = json.loads(stdout)
    except json.JSONDecodeError:
        return None
    if isinstance(envelope, dict):
        data = envelope.get("data")
        if isinstance(data, dict):
            return data
        # Some commands omit the envelope; accept the bare object too.
        if isinstance(envelope.get("counts"), dict):
            return envelope
    return None


def read_claude_mode(project_root):
    """Returns 'post-fix' (default) or 'block' from [claude].mode."""
    sbdb_toml = os.path.join(project_root, ".sbdb.toml")
    if not os.path.exists(sbdb_toml):
        return "post-fix"
    try:
        import tomllib
    except ImportError:
        return "post-fix"
    try:
        with open(sbdb_toml, "rb") as f:
            data = tomllib.load(f)
    except (OSError, ValueError):
        return "post-fix"
    mode = (data.get("claude") or {}).get("mode", "post-fix")
    return mode if mode in ("post-fix", "block") else "post-fix"


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
    # Drain stdin so the harness doesn't block waiting for us; we don't
    # actually need the event payload.
    try:
        sys.stdin.read()
    except Exception:
        pass
    main()
