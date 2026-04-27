# Repository workflow rules

These rules are mandatory for any work in this repo, including AI-assisted work.

## Issue-first workflow

Every change must be tracked by a GitHub issue. There are no exceptions.

1. **Before writing code, open an issue.** The issue captures the user's request: the problem, the proposed direction, and any design discussion. If the user asks for "feature X" or "fix Y", the first action is `gh issue create`, not editing code.
2. **One issue per logical change.** Don't bundle unrelated work under one issue. If a discussion spawns multiple changes, open separate issues and link them.
3. **Apply labels at issue-creation time.** Labels must reflect the change type (see mapping below). An unlabeled issue is incomplete.
4. **The issue title is the source of truth for the eventual commit prefix.** Title written as `feat: …` becomes commit `feat(scope): …` and PR title `feat(scope): …`.

## PR-issue linkage

Every PR must link to its issue using a closing keyword in the PR body:

```
Closes #<issue-number>
```

(`Closes`, `Fixes`, or `Resolves` — all auto-close the issue on merge.)

A PR with no linked issue is rejected. If you discover mid-work that a PR is doing more than one issue's worth of change, split it.

## Label and commit-prefix mapping

The commit prefix on the merge commit, the PR title prefix, and the issue label must all agree. Use this mapping:

| Conventional-commit prefix | Issue / PR label    | When to use                                                  |
|----------------------------|---------------------|--------------------------------------------------------------|
| `feat:`                    | `feat`              | New user-visible capability                                  |
| `fix:`                     | `fix`               | Bug fix to existing behavior                                 |
| `refactor:`                | `refactor`          | Internal restructure, no behavior change                     |
| `docs:`                    | `documentation`     | Docs-only change (README, spec, comments)                    |
| `chore:`                   | (no label needed)   | Build, tooling, deps                                         |
| `ci:`                      | (no label needed)   | CI workflow changes                                          |
| breaking change of any kind | `breaking-change`  | Add **in addition** to the type label above when the change requires migration (wire format, CLI flags, schema fields). PR title gets `!` (e.g. `feat!: …`) and the body must include a `BREAKING CHANGE:` footer. |

When the type maps to one of GitHub's default labels, prefer the convention-named alias for consistency with commit prefixes (`feat` over `enhancement`, `fix` over `bug`). Either is accepted on existing issues.

## Worked example

User says: "the event log should also record an actor's IP address."

1. Open an issue: title `feat: record actor IP on emitted events`, label `feat`. Body explains the why and any open questions.
2. Branch off `main`: `git checkout -b feat/event-actor-ip`.
3. Implement, with the commit message `feat(events): record actor IP on emitted events`.
4. Open the PR with title matching the commit, body including `Closes #<issue-number>` and a Test plan section.
5. On merge, the issue auto-closes; release-please consumes the conventional-commit prefix for the next changelog entry.

## Why these rules

- Issues are the durable record of *why* a change was made. Commit history shows *what*; the issue shows the conversation behind it.
- Labels feed downstream automation (release notes, dashboards) and let humans filter the issue tracker without reading every title.
- Linked PR/issue pairs collapse cleanly in GitHub's UI and produce coherent merge-commit metadata for release-please's changelog generation.

## Ground rules for collaborators (including AI assistants)

- If a request arrives without an issue, your first response is to open one. Do not start editing code, do not start exploring, do not start a branch.
- If you would naturally produce multiple PRs for one task, open multiple issues first.
- If existing code review or follow-up work surfaces a new concern, open a new issue rather than expanding the current PR's scope.
- A PR description that doesn't link an issue is a defect — fix it before requesting review.
