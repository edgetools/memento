---
name: memento-dream
description: Clean up and strengthen the memento brain — consolidate duplicates, fix orphans, improve cross-links
disable-model-invocation: true
---

You are doing a maintenance pass on the memento brain. Work through each task below. This is deliberate cleanup work — be conservative about deleting anything, but be aggressive about linking and consolidating.

## Task 1: Find orphaned pages

Get a complete picture of the brain first:
- Use `list_pages sort_by: "least_linked"` to surface orphan candidates at the top — pages with zero or few inbound links
- `list_pages` is paginated; keep calling it until you've seen all pages
- For each candidate, check `linked_from` in `get_page` results to confirm zero inbound links

Before deciding what to do with each orphan, check its `last_updated` timestamp (returned by `get_page`):
- **Recently updated**: the page was likely just created and hasn't been wired into the graph yet — find 2–3 pages that should reference it and add `[[wikilinks]]` via `patch_page`
- **Old and unlinked**: the page has been isolated for a long time — it's more likely obsolete, a merge candidate, or something that was captured and never followed up on

For each orphan, decide:
- **Link it**: if it's a real concept worth keeping, find pages that should reference it and add `[[wikilinks]]`. To find linking candidates, `search` with the orphan's title or first paragraph — semantic search will surface pages that discuss the same concept even when they use different wording.
- **Merge it**: if its content belongs on another page, append it there and delete the orphan
- **Delete it**: if the concept is obsolete or irrelevant, use `delete_page`

## Task 2: Consolidate duplicates

Look for pages that re-explain a concept already covered elsewhere:
- Query by concept, not term — `search("retry logic")` will surface pages titled "Exponential Backoff" or "Flaky Network Handling" even if neither contains the word "retry". Those are your consolidation candidates.
- Compare page content across the semantically-adjacent results
- If two pages cover the same concept, merge the content into the more canonical page and replace the redundant page's content with a redirect note pointing to the canonical page (or delete the duplicate outright)

## Task 3: Split oversized pages

Pages that have grown very long may be covering multiple distinct concepts:
- Read the full page and identify logical sub-topics
- If a sub-topic is self-contained and referenced elsewhere, extract it into its own page with `write_page`
- Replace the extracted section with a brief summary and a `[[link]]` to the new page
- Use `patch_page` for the replacement

## Task 4: Strengthen cross-links

For each major concept page, check whether related pages link to it:
- Read the page's `links_to` and `linked_from` fields
- Query by concept to find pages that discuss the topic but don't link to it — semantic search will surface paraphrased discussions the page author didn't think to link. If the concept is "Retry Backoff Strategy", `search("retrying after failures")` will turn up pages that should be linking but aren't.
- Add missing `[[wikilinks]]` via `patch_page`

## Task 5: Rename poorly named pages

Look for pages whose names are ambiguous, too terse, or no longer match what the page is actually about:
- A page named "Aggro" that is specifically about aggro from healing should be `rename_page` → "Aggro From Healing"
- A page whose scope expanded during editing may need a broader name
- Use `rename_page` — it atomically renames the page and updates all `[[wikilinks]]` across the brain that reference the old name
- Do not use delete + create for renames; that loses link graph integrity

## Task 6: Fix stale content

Work through the brain systematically from oldest to newest:
- Use `list_pages sort_by: "oldest"` to get pages ordered by last write timestamp, oldest first — this is your work queue
- `list_pages` is paginated; use `offset` to walk through it
- Read each page and update passages that contradict current understanding or reference outdated decisions
- After a rename, verify no stale references to the old name remain

## Rules

- Never delete a page without reading it first
- Prefer linking and merging over deletion — information is cheaper to keep than to recreate
- A maintenance session should leave the brain with fewer orphans, fewer duplicates, and more cross-links than it started with
- Commit at the end (or rely on `--auto-commit` if configured)
