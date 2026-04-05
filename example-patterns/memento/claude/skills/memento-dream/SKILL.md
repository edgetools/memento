---
name: memento-dream
description: Clean up and strengthen the memento brain — consolidate duplicates, fix orphans, improve cross-links
---

You are doing a maintenance pass on the memento brain. Work through each task below. This is deliberate cleanup work — be conservative about deleting anything, but be aggressive about linking and consolidating.

## Task 1: Find orphaned pages

Search for pages that are rarely or never linked to:
- Use `search` with broad terms to survey the brain
- For each page you find, check `linked_from` in `get_page` results
- Pages with zero inbound links are orphan candidates

For each orphan, decide:
- **Link it**: if it's a real concept, find 2–3 pages that should reference it and add `[[wikilinks]]` via `patch_page`
- **Merge it**: if its content belongs on another page, append it there and delete the orphan
- **Delete it**: if the concept is obsolete or irrelevant, use `delete_page`

## Task 2: Consolidate duplicates

Look for pages that re-explain a concept already covered elsewhere:
- Search for similar terms and compare page content
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
- Search for pages that discuss the concept but don't link to it
- Add missing `[[wikilinks]]` via `patch_page`

## Task 5: Rename poorly named pages

Look for pages whose names are ambiguous, too terse, or no longer match what the page is actually about:
- A page named "Aggro" that is specifically about aggro from healing should be `rename_page` → "Aggro From Healing"
- A page whose scope expanded during editing may need a broader name
- Use `rename_page` — it atomically renames the page and updates all `[[wikilinks]]` across the brain that reference the old name
- Do not use delete + create for renames; that loses link graph integrity

## Task 6: Fix stale content

Look for pages that reference outdated decisions or terminology:
- Update passages that contradict current understanding
- After a rename, verify no stale references to the old name remain

## Rules

- Never delete a page without reading it first
- Prefer linking and merging over deletion — information is cheaper to keep than to recreate
- A maintenance session should leave the brain with fewer orphans, fewer duplicates, and more cross-links than it started with
- Commit at the end (or rely on `--auto-commit` if configured)
