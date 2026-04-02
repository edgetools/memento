# Change Request: Add `list_pages` Tool

## Motivation

The dream and recall skills rely on search as the primary discovery mechanism,
but search requires the agent to know what to look for. Orphaned pages and
underlinked concepts are exactly the ones an agent won't think to search for —
they fall through the cracks precisely because nothing points to them. A
`list_pages` tool sorted by inbound link count gives dream sessions a systematic
entry point into the graph's edges, and gives recall sessions a way to survey
hub concepts before diving into a topic.

## Behavioral Changes

### 1. New `list_pages` tool

A new MCP tool is added. It returns a paginated, sorted list of page names with
no snippets or content — names only.

**Input fields (all optional):**

- `sort_by`: one of `"alphabetical"` (default), `"least_linked"`,
  `"most_linked"`. Controls the order of results.
- `limit`: integer, defaults to 50. Maximum number of page names to return.
- `offset`: integer, defaults to 0. Number of pages to skip before returning
  results. Used for pagination.
- `filter`: array of strings. When provided, only pages whose names contain
  ALL of the given keywords are included (case-insensitive substring match,
  AND semantics). This is name filtering, not semantic search.

**Output fields:**

- `pages`: array of page name strings, in the requested sort order, after
  filter and pagination are applied.
- `total`: integer count of pages matching the filter (before pagination).
  Allows the agent to determine whether more pages remain.
- `offset`: echoes the requested offset.
- `limit`: echoes the requested limit.

### 2. Sort behavior

- **`alphabetical`**: page names sorted A–Z, case-insensitive.
- **`least_linked`**: pages sorted ascending by inbound link count (pages that
  the fewest other pages link to appear first). Pages with zero inbound links
  appear at the top.
- **`most_linked`**: pages sorted descending by inbound link count (hub pages
  appear first). Ties in link count are broken alphabetically.

Inbound link count is derived from the existing bidirectional link graph already
maintained in memory. No new indexing is required.

### 3. Filter behavior

`filter` is a name-only substring match. It does not query the search index,
does not use BM25 or trigram matching, and does not consider page body content.
A page is included if and only if its name contains every keyword in the filter
array as a substring (case-insensitive). An empty filter array (or omitted
filter) includes all pages.

### 4. Pagination behavior

`total` always reflects the count of pages matching the filter, regardless of
`limit` and `offset`. An agent can walk the full list by incrementing `offset`
by `limit` until `offset >= total`.

## Affected Tools

No existing tools are modified. This is a purely additive change.

## Affected Internal Components

- **`tools/list_pages.go`**: New file implementing the tool handler.
- **`main.go`**: Tool registration for the new handler.
- The existing in-memory link graph (used for `least_linked` / `most_linked`
  sorting) is read but not modified.

## What Does NOT Change

- All existing tool input/output schemas are unaffected.
- The search pipeline, BM25 index, trigram index, and link graph are unaffected.
- No new indexing structures are required — link counts come from the existing
  graph.
- Git auto-commit behavior is unaffected (this tool is read-only).

## Test Case Ideas

### Sort order

- With a known set of pages and link relationships, `sort_by: "least_linked"`
  returns pages in ascending inbound link count order.
- `sort_by: "most_linked"` returns pages in descending inbound link count order.
- `sort_by: "alphabetical"` returns pages in A–Z order, case-insensitively.
- Ties in `least_linked` / `most_linked` are broken alphabetically.
- A page with zero inbound links appears before pages with one or more inbound
  links under `least_linked`.

### Filter

- `filter: ["combat"]` returns only pages whose names contain "combat"
  (case-insensitive).
- `filter: ["combat", "spell"]` returns only pages whose names contain both
  "combat" and "spell".
- `filter: ["COMBAT"]` matches the same pages as `filter: ["combat"]`.
- `filter: []` (empty array) returns all pages, same as omitting filter.
- A filter that matches no page names returns `pages: []` and `total: 0`.

### Pagination

- `limit: 2, offset: 0` on a 5-page brain returns the first 2 pages and
  `total: 5`.
- `limit: 2, offset: 2` returns pages 3–4 and `total: 5`.
- `limit: 2, offset: 4` returns page 5 only and `total: 5`.
- `offset >= total` returns `pages: []`.
- `total` reflects the filtered count, not the total page count, when a filter
  is applied.

### Defaults

- Calling `list_pages` with no arguments returns up to 50 pages sorted
  alphabetically.
- A brain with fewer than 50 pages returns all of them with the default limit.

### Edge cases

- An empty brain returns `pages: []` and `total: 0`.
- A brain with exactly one page returns that page with `total: 1`.
- Pages with identical inbound link counts are returned in a stable,
  deterministic order (alphabetical tiebreak).
- `filter` combined with `sort_by: "least_linked"` applies both correctly —
  only filtered pages are ranked and returned.
