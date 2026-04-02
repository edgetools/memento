# Change Request: Deduplicate linked pages in search response

## Problem

The `search` tool embeds full `linked_pages` entries (page name, snippet, line)
under every result. When the same page is linked from multiple results, its
snippet is repeated verbatim each time. In a real brain with interconnected
pages, this causes significant duplication in the response — wasting tokens and
making the output harder to scan.

## Desired behavior

The search response structure changes from:

```json
{
  "results": [
    {
      "page": "A",
      "relevance": 1,
      "snippet": "...",
      "line": 2,
      "linked_pages": [
        { "page": "X", "snippet": "...", "line": 2 },
        { "page": "Y", "snippet": "...", "line": 2 }
      ]
    },
    {
      "page": "B",
      "relevance": 0.9,
      "snippet": "...",
      "line": 5,
      "linked_pages": [
        { "page": "X", "snippet": "...", "line": 2 },
        { "page": "Z", "snippet": "...", "line": 2 }
      ]
    }
  ]
}
```

To:

```json
{
  "results": [
    {
      "page": "A",
      "relevance": 1,
      "snippet": "...",
      "line": 2,
      "linked_pages": ["X", "Y"]
    },
    {
      "page": "B",
      "relevance": 0.9,
      "snippet": "...",
      "line": 5,
      "linked_pages": ["X", "Z"]
    }
  ],
  "linked_page_details": [
    { "page": "X", "snippet": "...", "line": 2 },
    { "page": "Y", "snippet": "...", "line": 2 },
    { "page": "Z", "snippet": "...", "line": 2 }
  ]
}
```

### Rules

1. **`linked_pages` on each result becomes `[]string`** — just page names, no
   snippets or line numbers.

2. **`linked_page_details` is a new top-level field** on the response object,
   alongside `results`. It is a flat, deduplicated list of
   `{ page, snippet, line }` objects.

3. **Deduplication**: each linked page name appears at most once in
   `linked_page_details`, even if referenced by multiple results.

4. **Exclusion**: if a linked page already appears as a result in `results`
   (matched by page name, case-insensitive), it is **omitted** from
   `linked_page_details` entirely. It already has its own snippet in `results`.

5. **Ordering**: `linked_page_details` entries should appear in the order they
   are first encountered while iterating through results (stable, deterministic).

6. **Token budgeting**: the `max_tokens` budget should account for the total
   response size, including `linked_page_details`. The current approach of
   marshaling each result entry to JSON and counting words still works, but the
   `linked_page_details` entries also need to count against the budget. One
   approach: after building all results and collecting linked page details,
   marshal the full response and check total tokens. If over budget, remove
   results from the end (and their unique linked page details) until it fits.
   Alternatively, accumulate token counts for linked page details as they're
   added. Either approach is acceptable as long as the total response respects
   the budget.

## Files to change

### `tools/search.go`

This is the only file that needs to change. The modifications are:

1. **Replace `linkedPageEntry` struct usage in `resultEntry`**: change
   `LinkedPages []linkedPageEntry` to `LinkedPages []string` with json tag
   `"linked_pages"`.

2. **Add a new response-level type** for the linked page detail:
   keep the existing `linkedPageEntry` struct (or rename it) with fields
   `Page string`, `Snippet string`, `Line int`.

3. **Collect linked page details into a deduplicated list** as results are
   built:
   - Maintain a `seen` set (map[string]bool) of page names already in
     `linked_page_details` or in `results`.
   - Before adding a linked page to `linked_page_details`, check both sets.
   - Build the result's `linked_pages` as `[]string` (just names).

4. **Update the response struct** from:
   ```go
   resp := struct {
       Results []resultEntry `json:"results"`
   }{...}
   ```
   To:
   ```go
   resp := struct {
       Results           []resultEntry    `json:"results"`
       LinkedPageDetails []linkedPageEntry `json:"linked_page_details"`
   }{...}
   ```

5. **Update token budgeting** to account for the total response including
   `linked_page_details`.

### Tests

Any existing tests for the search tool that assert on the response JSON
structure will need to be updated to match the new shape. The linked_pages
field in results becomes `[]string`, and `linked_page_details` moves to the
top level.
