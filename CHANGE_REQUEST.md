# Change Request: Page Last-Updated Timestamps

## Summary

Expose the last-write timestamp for each page across the MCP tool surface. Agents interacting with memento have no shell or git access — the MCP is their only window into the brain. Without timestamps, an agent reading a page has no way to know whether a decision was captured last week or three years ago. This affects how confidently it should overwrite, how much it should trust the content as current, and how it should prioritize maintenance work.

---

## Changes

### 1. New field: `last_updated` on pages

Every page should carry a `last_updated` timestamp reflecting the last time its content was modified. The timestamp should be an ISO 8601 string in UTC (e.g. `"2026-04-09T14:32:00Z"`).

**What counts as a content-modifying write:**
- `write_page`
- `patch_page` (any operation)
- `rename_page` — (because renaming a page updates the header)

### Timestamp source

The MCP does not store timestamps internally. Instead, it derives `last_updated` from external sources in priority order:

1. **Git commit time** (when the content directory is inside a git repo): use the commit timestamp of the most recent commit that touched the page's file (`git log -1` for that file). This is the preferred source — commit timestamps survive `git clone` and `git pull`, making them reliable across machines.

2. **Filesystem mtime** (fallback): used when the content directory is not a git repo, or when a file exists in the directory but has never been committed (e.g. a newly created page not yet staged). Filesystem mtime is reliable for single-machine use but resets on a fresh clone, so it is a fallback only.

3. **Omit the field**: if neither source is available, `last_updated` should be omitted from the response rather than returning a zero value or fabricated date.

**Known limitation:** When `-auto-commit` is not enabled and the user edits a page outside the MCP (e.g. in Obsidian) without committing, the git-derived timestamp will reflect the last committed state rather than the actual last edit. This is intentional — uncommitted changes are in-flight and the last committed state is the most recent settled content. Engineers should document this behavior rather than treat it as a bug.

---

### 2. `get_page`: include `last_updated` in output

Add `last_updated` to the `get_page` response, for both full-page and line-range requests.

**Updated output (full page):**
```json
{
  "page": "Crowd Control",
  "content": "...",
  "total_lines": 142,
  "last_updated": "2024-11-03T09:15:42Z",
  "links_to": ["Enchanter", "Mez", "Pulling", "Root"],
  "linked_from": ["Party Composition", "Dungeon Strategy"]
}
```

**Updated output (line ranges):**
```json
{
  "page": "Crowd Control",
  "sections": [...],
  "total_lines": 142,
  "last_updated": "2024-11-03T09:15:42Z",
  "links_to": ["Enchanter", "Mez", "Pulling", "Root"],
  "linked_from": ["Party Composition", "Dungeon Strategy"]
}
```

---

### 3. `search`: include `last_updated` in results

Add `last_updated` to each result entry in `search` output. This lets agents calibrate trust in returned results during recall without needing a separate `get_page` call.

**Updated result entry:**
```json
{
  "page": "Crowd Control",
  "relevance": 0.87,
  "last_updated": "2024-11-03T09:15:42Z",
  "snippet": "...typically the [[Enchanter]] is assigned mez duty during pulls...",
  "line": 14,
  "linked_pages": ["Enchanter", "Pulling"]
}
```

`last_updated` should also appear on entries in `linked_page_details`.

---

### 4. `list_pages`: two new `sort_by` values and `last_updated` in output

Add two new sort options:

- **`newest`**: pages sorted by `last_updated` descending — most recently written first. Useful for dream sessions to find pages that were recently added or edited but may not yet have been linked into the graph.
- **`oldest`**: pages sorted by `last_updated` ascending — least recently written first. Useful for dream sessions doing stale-content review, working through the oldest pages systematically.

**Updated `sort_by` values (full list):**
- `alphabetical` (default)
- `least_linked`
- `most_linked`
- `newest`
- `oldest`

**Output format change:**

The current output returns `pages` as a flat array of name strings. To make timestamps available without a separate `get_page` call for every entry, change `pages` to an array of objects when `sort_by` is `newest` or `oldest`. For the existing sort modes (`alphabetical`, `least_linked`, `most_linked`), the output format should remain as a flat array of name strings to preserve token efficiency.

**Output when `sort_by` is `newest` or `oldest`:**
```json
{
  "pages": [
    { "page": "Crowd Control", "last_updated": "2026-04-01T10:00:00Z" },
    { "page": "Enchanter", "last_updated": "2025-09-14T08:44:21Z" },
    { "page": "Pulling", "last_updated": "2023-02-28T17:03:55Z" }
  ],
  "total": 247,
  "offset": 0,
  "limit": 50
}
```

**Output when `sort_by` is `alphabetical`, `least_linked`, or `most_linked` (unchanged):**
```json
{
  "pages": ["Crowd Control", "Enchanter", "Pulling"],
  "total": 247,
  "offset": 0,
  "limit": 50
}
```

---

## Motivation by Use Case

**kb-update (read before write):** An agent reading a page before updating it can now see when it was last written. A page last updated years ago can be freely overwritten; a page updated recently warrants more careful reading to understand why it was written that way.

**memento-dream / kb-dream (stale content review):** `list_pages sort_by: "oldest"` gives a systematic entry point into the stalest content. Without this, Task 6 (stale content) depends on the agent noticing staleness signals in prose — which is unreliable and unscalable as the brain grows.

**memento-dream (new orphan vs. old orphan):** Combining `least_linked` with a subsequent `newest` scan lets a dream session distinguish pages that are newly written but not yet integrated from pages that have been isolated for years and may be obsolete. These require different actions (link vs. archive/delete).

**memento-recall / kb-explore (trust calibration):** A search result carrying `last_updated` lets the agent weight recent captures more heavily than old ones when there is tension between multiple results. Without timestamps, the agent has no basis for preferring the more current understanding.

---

## Non-Changes

- No changes to `write_page`, `patch_page`, `rename_page`, or `delete_page` input schemas.
- No changes to search ranking. `last_updated` is surfaced as metadata only; it does not affect BM25 or graph boost scoring.
- No new tools. All changes are additions to existing tool outputs.
