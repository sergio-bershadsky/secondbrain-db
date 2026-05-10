#!/usr/bin/env python3
"""SessionStart hook: tell the user once that post-fix mode is the default.

Prints a single-line hint when:

  1. .sbdb.toml exists at the project root, AND
  2. The file has no [claude] section at all.

Once the user adds any [claude] key — `mode = "post-fix"` or
`mode = "block"` — this hook stays silent. There's no state file or
debouncing logic; the user editing `.sbdb.toml` is the natural off
switch. SessionStart fires once per Claude Code session, so a single
print is enough to surface the change without nagging.

The hint exists because flipping the default from `block` (the v2.x
behaviour) to `post-fix` is a soft behavioural change for existing
repos: the agent now edits docs/ directly. If a user wants the old
strict guard, the hint shows them the one-line opt-out.
"""

import json
import os
import sys


def main():
    project_root = find_project_root(os.getcwd())
    if not project_root:
        return  # not an sbdb project
    if has_claude_section(project_root):
        return  # user has made an explicit choice; stay silent

    print(json.dumps({
        "message": (
            "[sbdb] post-fix mode is now the default — Claude edits docs/ "
            "directly and sidecars are reconciled at end of turn. "
            "Add `[claude]\\nmode = \"block\"` to .sbdb.toml to keep the old "
            "strict guard."
        )
    }))


def has_claude_section(project_root):
    """True iff .sbdb.toml has any [claude] key (regardless of value)."""
    sbdb_toml = os.path.join(project_root, ".sbdb.toml")
    if not os.path.exists(sbdb_toml):
        return False
    try:
        import tomllib
    except ImportError:
        # Older Python — assume the user knows what they're doing and
        # don't show the hint. False positives (showing the hint when
        # the user actually has [claude]) would be more annoying than
        # missing it on legacy systems.
        return True
    try:
        with open(sbdb_toml, "rb") as f:
            data = tomllib.load(f)
    except (OSError, ValueError):
        # Malformed TOML — don't add to the user's confusion.
        return True
    claude = data.get("claude")
    return isinstance(claude, dict) and len(claude) > 0


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


if __name__ == "__main__":
    try:
        sys.stdin.read()
    except Exception:
        pass
    main()
