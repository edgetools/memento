package tools_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Response types --------------------------------------------------------

// listPagesResp mirrors the JSON output schema for list_pages.
type listPagesResp struct {
	Pages  []string `json:"pages"`
	Total  int      `json:"total"`
	Offset int      `json:"offset"`
	Limit  int      `json:"limit"`
}

// ---- Helpers ---------------------------------------------------------------

// indexOfPage returns the 0-based position of page in pages, or -1 if not found.
func indexOfPage(pages []string, page string) int {
	for i, p := range pages {
		if p == page {
			return i
		}
	}
	return -1
}

// ---- TestListPages ---------------------------------------------------------

func TestListPages(t *testing.T) {
	t.Parallel()

	// ------------------------------------------------------------------ sort

	t.Run("sort_alphabetical", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{"page": "Zebra Notes", "content": "Zebra content."})
		callTool(t, c, "write_page", map[string]any{"page": "Apple Guide", "content": "Apple content."})
		callTool(t, c, "write_page", map[string]any{"page": "Mango Tips", "content": "Mango content."})
		callTool(t, c, "write_page", map[string]any{"page": "Banana Facts", "content": "Banana content."})

		result := callTool(t, c, "list_pages", map[string]any{"sort_by": "alphabetical"})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		require.Equal(t, 4, resp.Total)
		require.Len(t, resp.Pages, 4)

		appleIdx := indexOfPage(resp.Pages, "Apple Guide")
		bananaIdx := indexOfPage(resp.Pages, "Banana Facts")
		mangoIdx := indexOfPage(resp.Pages, "Mango Tips")
		zebraIdx := indexOfPage(resp.Pages, "Zebra Notes")

		require.NotEqual(t, -1, appleIdx, "Apple Guide must be present")
		require.NotEqual(t, -1, bananaIdx, "Banana Facts must be present")
		require.NotEqual(t, -1, mangoIdx, "Mango Tips must be present")
		require.NotEqual(t, -1, zebraIdx, "Zebra Notes must be present")

		assert.Less(t, appleIdx, bananaIdx, "Apple Guide must precede Banana Facts (A < B)")
		assert.Less(t, bananaIdx, mangoIdx, "Banana Facts must precede Mango Tips (B < M)")
		assert.Less(t, mangoIdx, zebraIdx, "Mango Tips must precede Zebra Notes (M < Z)")
	})

	t.Run("sort_alphabetical_case_insensitive", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// "zebra notes" (lowercase first letter) should sort after "Apple Guide"
		// and "mango tips" case-insensitively, not before them via ASCII order.
		callTool(t, c, "write_page", map[string]any{"page": "zebra notes", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Apple Guide", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "mango tips", "content": "Content."})

		result := callTool(t, c, "list_pages", map[string]any{"sort_by": "alphabetical"})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		require.Len(t, resp.Pages, 3)

		appleIdx := indexOfPage(resp.Pages, "Apple Guide")
		mangoIdx := indexOfPage(resp.Pages, "mango tips")
		zebraIdx := indexOfPage(resp.Pages, "zebra notes")

		require.NotEqual(t, -1, appleIdx, "Apple Guide must be present")
		require.NotEqual(t, -1, mangoIdx, "mango tips must be present")
		require.NotEqual(t, -1, zebraIdx, "zebra notes must be present")

		assert.Less(t, appleIdx, mangoIdx,
			"Apple Guide must precede mango tips (case-insensitive A < M)")
		assert.Less(t, mangoIdx, zebraIdx,
			"mango tips must precede zebra notes (case-insensitive M < Z)")
	})

	t.Run("sort_most_linked", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// "Hub" will have 3 inbound links, "Medium" will have 1, "Orphan" will have 0.
		callTool(t, c, "write_page", map[string]any{"page": "Hub", "content": "Hub concept."})
		callTool(t, c, "write_page", map[string]any{"page": "Medium", "content": "Medium concept."})
		callTool(t, c, "write_page", map[string]any{"page": "Orphan", "content": "Orphan concept."})
		callTool(t, c, "write_page", map[string]any{"page": "Linker A", "content": "See [[Hub]]."})
		callTool(t, c, "write_page", map[string]any{"page": "Linker B", "content": "See [[Hub]]."})
		callTool(t, c, "write_page", map[string]any{"page": "Linker C", "content": "See [[Hub]] and [[Medium]]."})

		result := callTool(t, c, "list_pages", map[string]any{"sort_by": "most_linked"})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.Pages)

		hubIdx := indexOfPage(resp.Pages, "Hub")
		mediumIdx := indexOfPage(resp.Pages, "Medium")
		orphanIdx := indexOfPage(resp.Pages, "Orphan")

		require.NotEqual(t, -1, hubIdx, "Hub must be present")
		require.NotEqual(t, -1, mediumIdx, "Medium must be present")
		require.NotEqual(t, -1, orphanIdx, "Orphan must be present")

		assert.Less(t, hubIdx, mediumIdx,
			"Hub (3 inbound links) must appear before Medium (1 inbound link) in most_linked order")
		assert.Less(t, mediumIdx, orphanIdx,
			"Medium (1 inbound link) must appear before Orphan (0 inbound links) in most_linked order")
	})

	t.Run("sort_least_linked", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// "Busy" will have 3 inbound links, "Quiet" will have 1, "Lonely" will have 0.
		callTool(t, c, "write_page", map[string]any{"page": "Busy", "content": "Busy concept."})
		callTool(t, c, "write_page", map[string]any{"page": "Quiet", "content": "Quiet concept."})
		callTool(t, c, "write_page", map[string]any{"page": "Lonely", "content": "Lonely concept."})
		callTool(t, c, "write_page", map[string]any{"page": "Ref A", "content": "See [[Busy]]."})
		callTool(t, c, "write_page", map[string]any{"page": "Ref B", "content": "See [[Busy]]."})
		callTool(t, c, "write_page", map[string]any{"page": "Ref C", "content": "See [[Busy]] and [[Quiet]]."})

		result := callTool(t, c, "list_pages", map[string]any{"sort_by": "least_linked"})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.Pages)

		busyIdx := indexOfPage(resp.Pages, "Busy")
		quietIdx := indexOfPage(resp.Pages, "Quiet")
		lonelyIdx := indexOfPage(resp.Pages, "Lonely")

		require.NotEqual(t, -1, busyIdx, "Busy must be present")
		require.NotEqual(t, -1, quietIdx, "Quiet must be present")
		require.NotEqual(t, -1, lonelyIdx, "Lonely must be present")

		assert.Less(t, lonelyIdx, quietIdx,
			"Lonely (0 inbound links) must appear before Quiet (1 inbound link) in least_linked order")
		assert.Less(t, quietIdx, busyIdx,
			"Quiet (1 inbound link) must appear before Busy (3 inbound links) in least_linked order")
	})

	t.Run("sort_least_linked_zero_inbound_first", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Orphan has zero inbound links; Linked has at least one.
		callTool(t, c, "write_page", map[string]any{"page": "Linked Page", "content": "Linked content."})
		callTool(t, c, "write_page", map[string]any{"page": "Orphan Page", "content": "Orphan content."})
		callTool(t, c, "write_page", map[string]any{"page": "Pointer", "content": "See [[Linked Page]]."})

		result := callTool(t, c, "list_pages", map[string]any{
			"sort_by": "least_linked",
			"filter":  []string{"Page"},
		})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		require.Len(t, resp.Pages, 2, "filter should return only Linked Page and Orphan Page")

		orphanIdx := indexOfPage(resp.Pages, "Orphan Page")
		linkedIdx := indexOfPage(resp.Pages, "Linked Page")

		require.NotEqual(t, -1, orphanIdx, "Orphan Page must be present")
		require.NotEqual(t, -1, linkedIdx, "Linked Page must be present")

		assert.Less(t, orphanIdx, linkedIdx,
			"Orphan Page (0 inbound links) must appear before Linked Page (1 inbound link) in least_linked order")
	})

	t.Run("sort_link_count_tiebreak_alphabetical", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Three pages each with exactly one inbound link — tiebreak must be alphabetical.
		callTool(t, c, "write_page", map[string]any{"page": "Zeta Topic", "content": "Zeta content."})
		callTool(t, c, "write_page", map[string]any{"page": "Alpha Topic", "content": "Alpha content."})
		callTool(t, c, "write_page", map[string]any{"page": "Mu Topic", "content": "Mu content."})
		callTool(t, c, "write_page", map[string]any{"page": "Link To Zeta", "content": "See [[Zeta Topic]]."})
		callTool(t, c, "write_page", map[string]any{"page": "Link To Alpha", "content": "See [[Alpha Topic]]."})
		callTool(t, c, "write_page", map[string]any{"page": "Link To Mu", "content": "See [[Mu Topic]]."})

		for _, sortBy := range []string{"most_linked", "least_linked"} {
			sortBy := sortBy
			t.Run(sortBy, func(t *testing.T) {
				result := callTool(t, c, "list_pages", map[string]any{
					"sort_by": sortBy,
					"filter":  []string{"Topic"},
				})
				var resp listPagesResp
				parseJSON(t, result, &resp)

				require.Len(t, resp.Pages, 3, "filter should return only the three Topic pages")

				alphaIdx := indexOfPage(resp.Pages, "Alpha Topic")
				muIdx := indexOfPage(resp.Pages, "Mu Topic")
				zetaIdx := indexOfPage(resp.Pages, "Zeta Topic")

				require.NotEqual(t, -1, alphaIdx, "Alpha Topic must be present")
				require.NotEqual(t, -1, muIdx, "Mu Topic must be present")
				require.NotEqual(t, -1, zetaIdx, "Zeta Topic must be present")

				assert.Less(t, alphaIdx, muIdx,
					"Alpha Topic must precede Mu Topic (alphabetical tiebreak, sort_by=%s)", sortBy)
				assert.Less(t, muIdx, zetaIdx,
					"Mu Topic must precede Zeta Topic (alphabetical tiebreak, sort_by=%s)", sortBy)
			})
		}
	})

	// ---------------------------------------------------------------- filter

	t.Run("filter_single_keyword", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{"page": "Combat Spells", "content": "Offensive spells."})
		callTool(t, c, "write_page", map[string]any{"page": "Combat Tactics", "content": "Battle tactics."})
		callTool(t, c, "write_page", map[string]any{"page": "Healing Arts", "content": "Healing abilities."})

		result := callTool(t, c, "list_pages", map[string]any{"filter": []string{"Combat"}})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		assert.Equal(t, 2, resp.Total, "filter should match exactly 2 pages containing 'Combat'")
		require.Len(t, resp.Pages, 2)
		assert.Contains(t, resp.Pages, "Combat Spells")
		assert.Contains(t, resp.Pages, "Combat Tactics")
		assert.NotContains(t, resp.Pages, "Healing Arts", "Healing Arts must not appear in combat filter results")
	})

	t.Run("filter_multi_keyword_and_semantics", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{"page": "Combat Spells", "content": "Offensive spells."})
		callTool(t, c, "write_page", map[string]any{"page": "Combat Tactics", "content": "Battle tactics."})
		callTool(t, c, "write_page", map[string]any{"page": "Healing Spells", "content": "Healing magic."})

		result := callTool(t, c, "list_pages", map[string]any{
			"filter": []string{"Combat", "Spells"},
		})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		assert.Equal(t, 1, resp.Total, "AND filter should match only pages containing both 'Combat' and 'Spells'")
		require.Len(t, resp.Pages, 1)
		assert.Equal(t, "Combat Spells", resp.Pages[0])
	})

	t.Run("filter_case_insensitive", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{"page": "Combat Spells", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Combat Tactics", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Healing Arts", "content": "Content."})

		resultLower := callTool(t, c, "list_pages", map[string]any{"filter": []string{"combat"}})
		var respLower listPagesResp
		parseJSON(t, resultLower, &respLower)

		resultUpper := callTool(t, c, "list_pages", map[string]any{"filter": []string{"COMBAT"}})
		var respUpper listPagesResp
		parseJSON(t, resultUpper, &respUpper)

		assert.ElementsMatch(t, respLower.Pages, respUpper.Pages,
			"filter must be case-insensitive: 'COMBAT' and 'combat' should match the same pages")
		assert.Equal(t, respLower.Total, respUpper.Total,
			"total must be equal for case variants of the same filter keyword")
	})

	t.Run("filter_empty_returns_all", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{"page": "Alpha", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Beta", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Gamma", "content": "Content."})

		resultNoFilter := callTool(t, c, "list_pages", map[string]any{})
		var respNoFilter listPagesResp
		parseJSON(t, resultNoFilter, &respNoFilter)

		resultEmptyFilter := callTool(t, c, "list_pages", map[string]any{"filter": []string{}})
		var respEmptyFilter listPagesResp
		parseJSON(t, resultEmptyFilter, &respEmptyFilter)

		assert.Equal(t, 3, respNoFilter.Total)
		assert.ElementsMatch(t, respNoFilter.Pages, respEmptyFilter.Pages,
			"empty filter array must return the same pages as omitting the filter")
		assert.Equal(t, respNoFilter.Total, respEmptyFilter.Total,
			"empty filter array must produce the same total as omitting the filter")
	})

	t.Run("filter_no_match", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{"page": "Alpha", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Beta", "content": "Content."})

		result := callTool(t, c, "list_pages", map[string]any{
			"filter": []string{"xyznonexistentterm"},
		})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		assert.Empty(t, resp.Pages, "filter matching no pages must return an empty pages array")
		assert.Equal(t, 0, resp.Total, "filter matching no pages must return total: 0")
	})

	// -------------------------------------------------------------- pagination

	t.Run("pagination_first_page", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// 5 pages with clearly alphabetical names.
		for _, name := range []string{"Alpha Page", "Beta Page", "Charlie Page", "Delta Page", "Echo Page"} {
			callTool(t, c, "write_page", map[string]any{"page": name, "content": "Content."})
		}

		result := callTool(t, c, "list_pages", map[string]any{
			"sort_by": "alphabetical",
			"limit":   2,
			"offset":  0,
		})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		assert.Equal(t, 5, resp.Total, "total must reflect all matching pages regardless of pagination")
		assert.Equal(t, 0, resp.Offset, "offset must be echoed back")
		assert.Equal(t, 2, resp.Limit, "limit must be echoed back")
		require.Len(t, resp.Pages, 2, "limit:2 must return exactly 2 pages")
		assert.Equal(t, "Alpha Page", resp.Pages[0])
		assert.Equal(t, "Beta Page", resp.Pages[1])
	})

	t.Run("pagination_middle_page", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		for _, name := range []string{"Alpha Page", "Beta Page", "Charlie Page", "Delta Page", "Echo Page"} {
			callTool(t, c, "write_page", map[string]any{"page": name, "content": "Content."})
		}

		result := callTool(t, c, "list_pages", map[string]any{
			"sort_by": "alphabetical",
			"limit":   2,
			"offset":  2,
		})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		assert.Equal(t, 5, resp.Total)
		assert.Equal(t, 2, resp.Offset)
		assert.Equal(t, 2, resp.Limit)
		require.Len(t, resp.Pages, 2, "offset:2 with limit:2 must return pages 3-4")
		assert.Equal(t, "Charlie Page", resp.Pages[0])
		assert.Equal(t, "Delta Page", resp.Pages[1])
	})

	t.Run("pagination_last_page_partial", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		for _, name := range []string{"Alpha Page", "Beta Page", "Charlie Page", "Delta Page", "Echo Page"} {
			callTool(t, c, "write_page", map[string]any{"page": name, "content": "Content."})
		}

		result := callTool(t, c, "list_pages", map[string]any{
			"sort_by": "alphabetical",
			"limit":   2,
			"offset":  4,
		})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		assert.Equal(t, 5, resp.Total)
		assert.Equal(t, 4, resp.Offset)
		assert.Equal(t, 2, resp.Limit)
		require.Len(t, resp.Pages, 1, "offset:4 with limit:2 on 5 pages must return exactly 1 page")
		assert.Equal(t, "Echo Page", resp.Pages[0])
	})

	t.Run("pagination_offset_past_end", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		for _, name := range []string{"Alpha Page", "Beta Page", "Charlie Page"} {
			callTool(t, c, "write_page", map[string]any{"page": name, "content": "Content."})
		}

		result := callTool(t, c, "list_pages", map[string]any{
			"limit":  2,
			"offset": 10,
		})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		assert.Equal(t, 3, resp.Total, "total must still reflect all matching pages")
		assert.Empty(t, resp.Pages, "offset past total must return an empty pages array")
	})

	t.Run("pagination_total_reflects_filter_not_full_count", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// 3 pages match "Special", 2 pages do not.
		callTool(t, c, "write_page", map[string]any{"page": "Special Alpha", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Special Beta", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Special Gamma", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Other Delta", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Other Echo", "content": "Content."})

		result := callTool(t, c, "list_pages", map[string]any{
			"filter": []string{"Special"},
			"limit":  1,
			"offset": 0,
		})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		assert.Equal(t, 3, resp.Total,
			"total must reflect the filtered page count (3), not the total brain count (5)")
		require.Len(t, resp.Pages, 1, "limit:1 must return exactly 1 page")
	})

	t.Run("pagination_walk_all_pages", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		for _, name := range []string{"Page A", "Page B", "Page C", "Page D", "Page E"} {
			callTool(t, c, "write_page", map[string]any{"page": name, "content": "Content."})
		}

		// Walk the full list in steps of 2.
		var allPages []string
		offset := 0
		limit := 2
		for {
			result := callTool(t, c, "list_pages", map[string]any{
				"sort_by": "alphabetical",
				"limit":   limit,
				"offset":  offset,
			})
			var resp listPagesResp
			parseJSON(t, result, &resp)

			allPages = append(allPages, resp.Pages...)
			if offset >= resp.Total {
				break
			}
			offset += limit
			if offset >= resp.Total {
				break
			}
		}

		assert.Len(t, allPages, 5, "walking the full list must yield all 5 pages")
		assert.ElementsMatch(t, []string{"Page A", "Page B", "Page C", "Page D", "Page E"}, allPages)
	})

	// --------------------------------------------------------------- defaults

	t.Run("default_sort_is_alphabetical", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{"page": "Zebra", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Apple", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Mango", "content": "Content."})

		// Call with no arguments.
		result := callTool(t, c, "list_pages", map[string]any{})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		require.Len(t, resp.Pages, 3)

		appleIdx := indexOfPage(resp.Pages, "Apple")
		mangoIdx := indexOfPage(resp.Pages, "Mango")
		zebraIdx := indexOfPage(resp.Pages, "Zebra")

		require.NotEqual(t, -1, appleIdx)
		require.NotEqual(t, -1, mangoIdx)
		require.NotEqual(t, -1, zebraIdx)

		assert.Less(t, appleIdx, mangoIdx,
			"default sort must be alphabetical: Apple before Mango")
		assert.Less(t, mangoIdx, zebraIdx,
			"default sort must be alphabetical: Mango before Zebra")
	})

	t.Run("default_limit_is_fifty", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Write 60 pages — default limit of 50 should cap the result.
		for i := range 60 {
			callTool(t, c, "write_page", map[string]any{
				"page":    fmt.Sprintf("Default Limit Page %02d", i),
				"content": "Content.",
			})
		}

		result := callTool(t, c, "list_pages", map[string]any{})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		assert.Equal(t, 60, resp.Total,
			"total must reflect all 60 pages when no filter is applied")
		assert.LessOrEqual(t, len(resp.Pages), 50,
			"default limit must cap pages returned at 50")
		assert.Equal(t, 50, resp.Limit,
			"limit must be echoed back as 50 when using the default")
	})

	t.Run("default_limit_fewer_than_fifty_returns_all", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		for i := range 10 {
			callTool(t, c, "write_page", map[string]any{
				"page":    fmt.Sprintf("Small Brain Page %d", i),
				"content": "Content.",
			})
		}

		result := callTool(t, c, "list_pages", map[string]any{})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		assert.Equal(t, 10, resp.Total)
		assert.Len(t, resp.Pages, 10,
			"a brain with fewer than 50 pages must return all pages under the default limit")
	})

	t.Run("default_offset_is_zero", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		for _, name := range []string{"Alpha", "Beta", "Gamma"} {
			callTool(t, c, "write_page", map[string]any{"page": name, "content": "Content."})
		}

		result := callTool(t, c, "list_pages", map[string]any{})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		assert.Equal(t, 0, resp.Offset,
			"default offset must be 0 when not specified")
		assert.Equal(t, "Alpha", resp.Pages[0],
			"default offset:0 must start from the first page")
	})

	// ------------------------------------------------------------ edge cases

	t.Run("empty_brain", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		result := callTool(t, c, "list_pages", map[string]any{})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		assert.Empty(t, resp.Pages, "empty brain must return an empty pages array")
		assert.Equal(t, 0, resp.Total, "empty brain must return total: 0")
	})

	t.Run("single_page_brain", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{"page": "Only Page", "content": "Sole content."})

		result := callTool(t, c, "list_pages", map[string]any{})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		assert.Equal(t, 1, resp.Total)
		require.Len(t, resp.Pages, 1)
		assert.Equal(t, "Only Page", resp.Pages[0])
	})

	t.Run("stable_tiebreak_deterministic", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// All three pages have zero inbound links — tiebreak must be alphabetical and stable.
		callTool(t, c, "write_page", map[string]any{"page": "Isolated Zeta", "content": "No links."})
		callTool(t, c, "write_page", map[string]any{"page": "Isolated Alpha", "content": "No links."})
		callTool(t, c, "write_page", map[string]any{"page": "Isolated Mu", "content": "No links."})

		for _, sortBy := range []string{"least_linked", "most_linked"} {
			sortBy := sortBy
			t.Run(sortBy, func(t *testing.T) {
				result := callTool(t, c, "list_pages", map[string]any{
					"sort_by": sortBy,
					"filter":  []string{"Isolated"},
				})
				var resp listPagesResp
				parseJSON(t, result, &resp)

				require.Len(t, resp.Pages, 3)
				assert.Equal(t, "Isolated Alpha", resp.Pages[0],
					"alphabetical tiebreak for equal link counts must be stable (sort_by=%s)", sortBy)
				assert.Equal(t, "Isolated Mu", resp.Pages[1],
					"alphabetical tiebreak for equal link counts must be stable (sort_by=%s)", sortBy)
				assert.Equal(t, "Isolated Zeta", resp.Pages[2],
					"alphabetical tiebreak for equal link counts must be stable (sort_by=%s)", sortBy)
			})
		}
	})

	t.Run("filter_combined_with_least_linked", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Only "Spell" pages should be ranked; non-spell pages must be excluded even if they
		// have different link counts that would otherwise affect ordering.
		callTool(t, c, "write_page", map[string]any{"page": "Spell Hub", "content": "Central spell reference."})
		callTool(t, c, "write_page", map[string]any{"page": "Spell Orphan", "content": "An isolated spell."})
		callTool(t, c, "write_page", map[string]any{"page": "Unrelated Page", "content": "No spells here."})
		// Give Spell Hub 2 inbound links.
		callTool(t, c, "write_page", map[string]any{"page": "Ref X", "content": "See [[Spell Hub]]."})
		callTool(t, c, "write_page", map[string]any{"page": "Ref Y", "content": "See [[Spell Hub]]."})
		// Unrelated Page gets 3 inbound links — but it should not affect Spell ordering.
		callTool(t, c, "write_page", map[string]any{"page": "Ref W1", "content": "See [[Unrelated Page]]."})
		callTool(t, c, "write_page", map[string]any{"page": "Ref W2", "content": "See [[Unrelated Page]]."})
		callTool(t, c, "write_page", map[string]any{"page": "Ref W3", "content": "See [[Unrelated Page]]."})

		result := callTool(t, c, "list_pages", map[string]any{
			"sort_by": "least_linked",
			"filter":  []string{"Spell"},
		})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		assert.Equal(t, 2, resp.Total, "filter must exclude non-Spell pages from total")
		require.Len(t, resp.Pages, 2)
		assert.NotContains(t, resp.Pages, "Unrelated Page",
			"filter must exclude pages not matching the keyword")

		orphanIdx := indexOfPage(resp.Pages, "Spell Orphan")
		hubIdx := indexOfPage(resp.Pages, "Spell Hub")

		require.NotEqual(t, -1, orphanIdx, "Spell Orphan must be present")
		require.NotEqual(t, -1, hubIdx, "Spell Hub must be present")

		assert.Less(t, orphanIdx, hubIdx,
			"Spell Orphan (0 inbound links) must appear before Spell Hub (2 inbound links) in least_linked order")
	})

	t.Run("output_names_only_no_snippets", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Name Only Page",
			"content": "This body content must not appear in list_pages output.",
		})

		result := callTool(t, c, "list_pages", map[string]any{})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		// The response fields Pages/Total/Offset/Limit are all that should exist.
		// parseJSON into listPagesResp would silently drop unknown fields, but
		// the pages array must be strings only (no objects with snippet fields).
		require.Len(t, resp.Pages, 1)
		assert.Equal(t, "Name Only Page", resp.Pages[0],
			"pages array must contain plain name strings only")
	})

	t.Run("reflects_writes", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{"page": "First Page", "content": "Content."})

		resultBefore := callTool(t, c, "list_pages", map[string]any{})
		var respBefore listPagesResp
		parseJSON(t, resultBefore, &respBefore)
		require.Equal(t, 1, respBefore.Total)

		callTool(t, c, "write_page", map[string]any{"page": "Second Page", "content": "Content."})

		resultAfter := callTool(t, c, "list_pages", map[string]any{})
		var respAfter listPagesResp
		parseJSON(t, resultAfter, &respAfter)

		assert.Equal(t, 2, respAfter.Total, "list_pages must reflect newly written pages immediately")
		assert.Contains(t, respAfter.Pages, "Second Page")
	})

	t.Run("reflects_deletes", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{"page": "Keep Page", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Delete Page", "content": "Content."})

		callTool(t, c, "delete_page", map[string]any{"page": "Delete Page"})

		result := callTool(t, c, "list_pages", map[string]any{})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		assert.Equal(t, 1, resp.Total, "list_pages must reflect deletions immediately")
		assert.NotContains(t, resp.Pages, "Delete Page", "deleted page must not appear in list_pages")
	})
}
