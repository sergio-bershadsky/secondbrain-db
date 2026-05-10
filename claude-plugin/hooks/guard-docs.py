#!/usr/bin/env python3
"""PreToolUse hook: block direct AI edits under docs/ in sbdb-managed repos
(only when [claude] mode = "block" is set in .sbdb.toml).

The default is post-fix: direct edits are allowed and a Stop hook reconciles
sidecars at session end via `sbdb doctor heal`. Users who want strict
real-time integrity checks opt back in with `[claude] mode = "block"`, in
which case this hook rejects Write/Edit/MultiEdit/NotebookEdit targeting
docs/ and scans Bash commands for file-mutation patterns targeting docs/.

If sbdb is not installed, the block message includes install instructions.
"""

import json
import os
import re
import sys


BASH_MUTATION_RE = re.compile(
    r"""
    (?:^|[\s;&|`(])                                   # start or shell separator
    (?:
        (?:rm|mv|cp|tee|touch|mkdir|rmdir|chmod|chown|ln)\b
      | (?:sed|gsed|perl)\s+[^|;&]*?-i\b
      | awk\s+[^|;&]*?-i\s+inplace\b
    )
    """,
    re.VERBOSE,
)

REDIRECT_RE = re.compile(r"(?:>>?|\|\s*tee(?:\s+-a)?)\s+[\"']?([^\s\"';&|]+)")
TOKEN_RE = re.compile(r"[\"']?([^\s\"';&|<>]+)")


def main():
    try:
        event = json.loads(sys.stdin.read())
    except (json.JSONDecodeError, ValueError):
        return

    tool_name = event.get("tool_name", "")
    tool_input = event.get("tool_input", {}) or {}

    targets = collect_targets(tool_name, tool_input)
    if not targets:
        return

    for target in targets:
        project_root = find_project_root(target)
        if not project_root:
            continue
        if not is_under_docs(target, project_root):
            continue
        if read_claude_mode(project_root) != "block":
            continue
        emit_block(project_root)
        return


def collect_targets(tool_name, tool_input):
    """Return a list of absolute paths that the tool call would mutate."""
    if tool_name in ("Write", "Edit", "MultiEdit"):
        p = tool_input.get("file_path")
        return [os.path.abspath(p)] if p else []

    if tool_name == "NotebookEdit":
        p = tool_input.get("notebook_path") or tool_input.get("file_path")
        return [os.path.abspath(p)] if p else []

    if tool_name == "Bash":
        cmd = tool_input.get("command", "") or ""
        return bash_targets(cmd)

    return []


def bash_targets(cmd):
    """Heuristically extract paths a bash command would mutate.

    Errs on the side of caution: if the command looks like a mutation and
    mentions a docs/ path anywhere in its tokens, block it.
    """
    targets = []
    cwd = os.getcwd()

    is_mutation = bool(BASH_MUTATION_RE.search(cmd))

    # Shell redirects always write to their target, regardless of leading cmd.
    for m in REDIRECT_RE.finditer(cmd):
        targets.append(_resolve(m.group(1), cwd))

    if is_mutation:
        # Scan all tokens that look like paths containing "docs".
        for m in TOKEN_RE.finditer(cmd):
            tok = m.group(1)
            if "docs" in tok and ("/" in tok or tok == "docs"):
                targets.append(_resolve(tok, cwd))

    return targets


def _resolve(path, cwd):
    if os.path.isabs(path):
        return os.path.normpath(path)
    return os.path.normpath(os.path.join(cwd, path))


def is_under_docs(abs_path, project_root):
    try:
        rel = os.path.relpath(abs_path, project_root)
    except ValueError:
        return False
    if rel.startswith(".."):
        return False
    parts = rel.split(os.sep)
    return len(parts) > 0 and parts[0] == "docs"


def find_project_root(file_path):
    """Walk up from the file path looking for .sbdb.toml."""
    directory = os.path.dirname(os.path.abspath(file_path))
    if not directory:
        directory = os.getcwd()
    for _ in range(10):
        if os.path.exists(os.path.join(directory, ".sbdb.toml")):
            return directory
        parent = os.path.dirname(directory)
        if parent == directory:
            break
        directory = parent
    return None


def read_claude_mode(project_root):
    """Returns 'post-fix' (default) or 'block', read from [claude].mode.

    Falls back to the post-fix default on any parse error, missing file, or
    older Python without tomllib. The hook is best-effort: if we can't tell
    what mode the user wants, the safer choice is to stay out of the way.
    """
    sbdb_toml = os.path.join(project_root, ".sbdb.toml")
    if not os.path.exists(sbdb_toml):
        return "post-fix"
    try:
        import tomllib  # Python 3.11+
    except ImportError:
        return "post-fix"
    try:
        with open(sbdb_toml, "rb") as f:
            data = tomllib.load(f)
    except (OSError, ValueError):
        return "post-fix"
    mode = (data.get("claude") or {}).get("mode", "post-fix")
    return mode if mode in ("post-fix", "block") else "post-fix"


def find_sbdb():
    """Find the sbdb binary in PATH or common locations."""
    for path_dir in os.environ.get("PATH", "").split(os.pathsep):
        candidate = os.path.join(path_dir, "sbdb")
        if os.path.isfile(candidate) and os.access(candidate, os.X_OK):
            return candidate

    home = os.path.expanduser("~")
    for candidate in [
        os.path.join(home, "go", "bin", "sbdb"),
        "/usr/local/bin/sbdb",
    ]:
        if os.path.isfile(candidate) and os.access(candidate, os.X_OK):
            return candidate

    return None


def emit_block(project_root):
    sbdb = find_sbdb()

    if sbdb:
        reason = (
            f"This repo opted into strict block-mode "
            f"(.sbdb.toml at {project_root} has `[claude] mode = \"block\"`). "
            f"Direct edits to docs/ are blocked because the .md file and its "
            f"sibling <id>.yaml integrity sidecar must move in lockstep.\n\n"
            f"Load the `secondbrain-db-edit` skill for the full workflow. "
            f"Quick reference:\n"
            f"  # Body rewrite — write new body to a file, then:\n"
            f"  sbdb update -s <schema> --id <id> --content-file <path>\n\n"
            f"  # Frontmatter-only tweaks:\n"
            f"  sbdb update -s <schema> --id <id> --field key=value\n\n"
            f"  # Combined body+frontmatter via JSON on stdin:\n"
            f"  echo '{{\"content\":\"...\",\"key\":\"value\"}}' \\\n"
            f"    | sbdb update -s <schema> --id <id> --input -\n\n"
            f"  # Create / delete:\n"
            f"  sbdb create -s <schema> --input -   # JSON on stdin\n"
            f"  sbdb delete -s <schema> --id <id> --yes\n\n"
            f"After an unintentional direct edit, "
            f"`sbdb doctor heal --i-meant-it --id <id>` reconciles the sidecar.\n\n"
            f"To switch to the default post-fix workflow (Claude edits docs/ "
            f"directly, sidecars are reconciled at session end), remove the "
            f"`mode = \"block\"` line from .sbdb.toml."
        )
    else:
        reason = (
            f"Direct edits to docs/ are not allowed in sbdb-managed repos "
            f"(.sbdb.toml at {project_root}), and the `sbdb` CLI was not "
            f"found in PATH or ~/go/bin.\n\n"
            f"Install with:\n"
            f"  go install github.com/sergio-bershadsky/secondbrain-db@latest\n"
            f"Then ensure $(go env GOPATH)/bin is on your PATH.\n"
            f"After installing, retry via `sbdb create / update / delete` "
            f"(see `sbdb --help`)."
        )

    print(json.dumps({
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": reason,
        },
        "decision": "block",
        "reason": reason,
    }))
    sys.exit(0)


if __name__ == "__main__":
    main()
