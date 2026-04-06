# Change Request: Auto-Commit Must Stage Only Files Modified by Memento

## Problem

When `-auto-commit` is enabled, the auto-commit logic runs `git add -A` before every commit. This stages **all** changes in the repository — not just the files memento modified. When the memento content directory is a subdirectory of a larger git repo (a common setup), unrelated staged or unstaged changes get swept into memento's commits. This corrupts the git history by mixing memento-managed content with human changes that haven't been reviewed or approved.

## Root Cause

`autoCommitter.commit` in `tools/tools.go` uses `git add -A` unconditionally. The method receives only a commit message and has no knowledge of which specific files were modified by the tool call that invoked it.

## Goal

Change the auto-commit logic so that each commit stages **only the specific files that the tool call modified** — no more, no less.

## Scope

All four write tools (`write_page`, `patch_page`, `rename_page`, `delete_page`) must pass the exact set of file paths they modified to the commit step, and the commit step must stage only those files.

Key cases to handle correctly:
- `write_page` and `patch_page` each modify exactly one file.
- `delete_page` removes exactly one file (staging a deletion).
- `rename_page` may modify multiple files: the renamed page itself plus every page whose wikilinks were rewritten. All of them must be staged in a single commit.

## Requirements

1. The commit step must use `git add -- <file1> <file2> ...` (explicit file paths) instead of `git add -A`.
2. File paths passed to the commit step must be absolute or correctly relative to the git repo root — whichever form `git add` requires when run from `contentDir`.
3. The behavior must be correct when `contentDir` is a subdirectory of a larger git repo. Changes outside `contentDir` must never be staged.
4. Existing behavior is preserved in all other respects: one commit per tool call, commit message format unchanged, `commit_failures` reporting unchanged, no commits when auto-commit is disabled.

## Testing

Tests must cover:
- Each write tool (`write_page`, `patch_page`, `rename_page`, `delete_page`) stages only its own file(s) when there are unrelated dirty files in the repository.
- `rename_page` stages all modified files (the renamed page + all rewritten linkers) but nothing else.
- The existing auto-commit and commit-failure test coverage continues to pass.

The test setup should create unrelated dirty files in the repo alongside the memento content directory and assert that those files are **not** included in memento's commits.

## Documentation

Update README and DESIGN to reflect the corrected behavior description of `-auto-commit`. Do not include historical decisions or changes, just ensure it reflects the current state after the changes.
