package tools_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Response types --------------------------------------------------------

type searchLinkedPage struct {
	Page    string `json:"page"`
	Snippet string `json:"snippet"`
	Line    int    `json:"line"`
}

type searchResult struct {
	Page        string             `json:"page"`
	Relevance   float64            `json:"relevance"`
	Snippet     string             `json:"snippet"`
	Line        int                `json:"line"`
	LinkedPages []searchLinkedPage `json:"linked_pages"`
}

type searchResp struct {
	Results []searchResult `json:"results"`
}

// ---- Helpers ---------------------------------------------------------------

// findSearchResult returns a pointer to the searchResult for the given page name,
// or nil if not present.
func findSearchResult(results []searchResult, page string) *searchResult {
	for i := range results {
		if results[i].Page == page {
			return &results[i]
		}
	}
	return nil
}

// searchResultPages extracts the Page field from each searchResult for easy
// membership assertions.
func searchResultPages(results []searchResult) []string {
	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Page
	}
	return names
}

// ---- TestSearch ------------------------------------------------------------

func TestSearch(t *testing.T) {
	t.Parallel()

	t.Run("finds_by_title", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "The enchanter is a utility class.",
		})
		// A second page that only mentions the term in the body.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Notes",
			"content": "Various notes including a brief mention of the enchanter role.",
		})

		result := callTool(t, c, "search", map[string]any{"query": "enchanter"})
		var resp searchResp
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.Results, "search should return results for a term matching a page title")
		assert.Equal(t, "Enchanter", resp.Results[0].Page,
			"page whose title is the search term must rank first (title weight is 10×)")
	})

	t.Run("finds_by_body", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Tactics",
			"content": "Mesmerize is a powerful CC technique used against casters.",
		})

		result := callTool(t, c, "search", map[string]any{"query": "mesmerize"})
		var resp searchResp
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.Results, "search should find pages with a match in the body")
		names := searchResultPages(resp.Results)
		assert.Contains(t, names, "Crowd Tactics")
	})

	t.Run("finds_by_wikilink", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter Mez",
			"content": "Mesmerize abilities of the enchanter class.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "CC relies on [[Enchanter Mez]] for primary crowd control.",
		})

		result := callTool(t, c, "search", map[string]any{"query": "enchanter mez"})
		var resp searchResp
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.Results, "search should find pages via wikilink targets")
		names := searchResultPages(resp.Results)
		// The page whose title matches should appear.
		assert.Contains(t, names, "Enchanter Mez")
		// The page with the matching wikilink should also surface.
		assert.Contains(t, names, "Crowd Control",
			"page containing [[Enchanter Mez]] wikilink should appear in results (3× wikilink weight)")
	})

	t.Run("ranked", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Strong body match: "enchanter" appears 3× in a focused page (no title match).
		// BM25 TF saturation means 3× gives roughly 1.5× the score of 1× at similar lengths,
		// keeping both pages within the 50% relevance threshold of each other.
		callTool(t, c, "write_page", map[string]any{
			"page":    "CC Roles",
			"content": "The enchanter is the primary CC class. Enchanter handles mez. An enchanter controls casters.",
		})
		// Weak body match: "enchanter" appears once in an unrelated page (no title match).
		callTool(t, c, "write_page", map[string]any{
			"page":    "General Notes",
			"content": "Various game notes. An enchanter was briefly seen.",
		})

		result := callTool(t, c, "search", map[string]any{"query": "enchanter"})
		var resp searchResp
		parseJSON(t, result, &resp)

		require.GreaterOrEqual(t, len(resp.Results), 2, "both pages should appear in results")

		ccResult := findSearchResult(resp.Results, "CC Roles")
		notesResult := findSearchResult(resp.Results, "General Notes")
		require.NotNil(t, ccResult, "CC Roles must appear in results")
		require.NotNil(t, notesResult, "General Notes must appear in results")

		assert.Greater(t, ccResult.Relevance, notesResult.Relevance,
			"page with more term occurrences must have higher relevance than a single body mention")

		// Results must be in descending relevance order.
		for i := 1; i < len(resp.Results); i++ {
			assert.GreaterOrEqualf(t, resp.Results[i-1].Relevance, resp.Results[i].Relevance,
				"results must be sorted by descending relevance: results[%d].Relevance=%f >= results[%d].Relevance=%f",
				i-1, resp.Results[i-1].Relevance, i, resp.Results[i].Relevance)
		}
	})

	t.Run("max_results_honored", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		for i := range 6 {
			callTool(t, c, "write_page", map[string]any{
				"page":    fmt.Sprintf("Mez Technique %d", i),
				"content": "Mez is a crowd control technique used by enchanters.",
			})
		}

		result := callTool(t, c, "search", map[string]any{
			"query":       "mez",
			"max_results": 3,
		})
		var resp searchResp
		parseJSON(t, result, &resp)

		assert.LessOrEqual(t, len(resp.Results), 3,
			"max_results must cap the number of returned results")
	})

	t.Run("default_max_results", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Write 15 pages all matching the same term to exceed the default cap.
		for i := range 15 {
			callTool(t, c, "write_page", map[string]any{
				"page":    fmt.Sprintf("Paladin Build %d", i),
				"content": "The paladin is a holy warrior class with healing abilities.",
			})
		}

		result := callTool(t, c, "search", map[string]any{"query": "paladin"})
		var resp searchResp
		parseJSON(t, result, &resp)

		assert.LessOrEqual(t, len(resp.Results), 10,
			"default max_results must be 10 when the parameter is omitted")
	})

	t.Run("max_tokens_limits", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Write 10 pages all matching the same term.
		for i := range 10 {
			callTool(t, c, "write_page", map[string]any{
				"page": fmt.Sprintf("Warrior Build %d", i),
				"content": "The warrior class is a frontline melee fighter. " +
					"Warriors use heavy armor and two-handed weapons in combat.",
			})
		}

		// Baseline: without a token budget, expect multiple results.
		resultFull := callTool(t, c, "search", map[string]any{
			"query":       "warrior",
			"max_results": 10,
		})
		var respFull searchResp
		parseJSON(t, resultFull, &respFull)
		require.NotEmpty(t, respFull.Results, "baseline search should return results")

		// With a very tight token budget, fewer results should be returned.
		// Each serialized result is at minimum ~10+ whitespace-split tokens
		// (page name, relevance, snippet words, line number, linked_pages).
		// A budget of 15 tokens should fit at most 1 result.
		resultLimited := callTool(t, c, "search", map[string]any{
			"query":      "warrior",
			"max_tokens": 15,
		})
		var respLimited searchResp
		parseJSON(t, resultLimited, &respLimited)

		assert.Less(t, len(respLimited.Results), len(respFull.Results),
			"max_tokens budget must reduce the number of results compared to no budget")
	})

	t.Run("has_snippets", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "The enchanter is a utility class specializing in mesmerize and crowd control.",
		})

		result := callTool(t, c, "search", map[string]any{"query": "enchanter"})
		var resp searchResp
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.Results, "search should return results")
		for _, r := range resp.Results {
			assert.NotEmpty(t, r.Snippet,
				"every search result must include a non-empty snippet (result page: %q)", r.Page)
		}
	})

	t.Run("has_line_numbers", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "The enchanter is a utility class.\nEnchanters excel at crowd control.",
		})

		result := callTool(t, c, "search", map[string]any{"query": "enchanter"})
		var resp searchResp
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.Results, "search should return results")
		for _, r := range resp.Results {
			assert.Greater(t, r.Line, 0,
				"every search result must have a positive 1-indexed line number (result page: %q)", r.Page)
		}
	})

	t.Run("has_linked_pages", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "The enchanter specializes in mesmerize and crowd control effects.",
		})
		// This page links to "Enchanter" via wikilink.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "CC relies on [[Enchanter]] for primary mez duties.",
		})

		result := callTool(t, c, "search", map[string]any{"query": "crowd control"})
		var resp searchResp
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.Results, "search should return results")

		ccResult := findSearchResult(resp.Results, "Crowd Control")
		require.NotNil(t, ccResult, "Crowd Control should appear as a direct result")

		require.NotEmpty(t, ccResult.LinkedPages,
			"a result with outbound wikilinks must include linked_pages in the response")

		var enchLinked *searchLinkedPage
		for i := range ccResult.LinkedPages {
			if ccResult.LinkedPages[i].Page == "Enchanter" {
				enchLinked = &ccResult.LinkedPages[i]
				break
			}
		}
		require.NotNil(t, enchLinked,
			"linked_pages must contain an entry for [[Enchanter]] (linked from Crowd Control)")
		assert.NotEmpty(t, enchLinked.Snippet,
			"linked page entry must include a non-empty snippet")
		assert.Greater(t, enchLinked.Line, 0,
			"linked page entry must include a positive line number")
	})

	t.Run("no_results", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "The enchanter handles crowd control.",
		})

		// Use a term guaranteed not to exist in any indexed content.
		result := callTool(t, c, "search", map[string]any{
			"query": "zxqvwjkpfmblxyzqqqquniqueterm",
		})
		var resp searchResp
		parseJSON(t, result, &resp)

		// Results must be an empty (non-nil) array, not a missing field.
		assert.Empty(t, resp.Results,
			"search with no matching pages must return an empty results array")
	})

	t.Run("empty_query_errors", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callToolExpectError(t, c, "search", map[string]any{
			"query": "",
		})
	})

	t.Run("relevance_threshold", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Strong match: page title IS "Enchanter" (10× weight) with body reinforcement.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "The enchanter is the primary mez class. Enchanter handles all crowd control.",
		})
		// Weak match: a very long page that mentions "enchanter" exactly once.
		// BM25 length normalization will heavily penalize this, putting it well
		// below the 50% relevance threshold.
		longBody := strings.Repeat(
			"This section covers many unrelated game mechanics and strategies. ",
			40,
		) + "An enchanter was briefly mentioned in passing."
		callTool(t, c, "write_page", map[string]any{
			"page":    "Long Treatise",
			"content": longBody,
		})

		result := callTool(t, c, "search", map[string]any{"query": "enchanter"})
		var resp searchResp
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.Results, "at least one result must be returned")
		names := searchResultPages(resp.Results)
		assert.Contains(t, names, "Enchanter",
			"strong match must survive the relevance threshold")
		assert.NotContains(t, names, "Long Treatise",
			"weak single-mention in a very long page must be filtered by the 50%% relevance threshold")
	})

	t.Run("reflects_writes", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Use a distinctive term unlikely to collide with any other test page.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Unique Written Page",
			"content": "This page uses the term flibbertigibbet as a unique marker.",
		})

		result := callTool(t, c, "search", map[string]any{"query": "flibbertigibbet"})
		var resp searchResp
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.Results,
			"search must find a page immediately after it is written")
		names := searchResultPages(resp.Results)
		assert.Contains(t, names, "Unique Written Page")
	})

	t.Run("reflects_deletes", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Deletable Page",
			"content": "This page uses the term snollygoster as a unique marker.",
		})

		// Confirm it is searchable before deletion.
		beforeResult := callTool(t, c, "search", map[string]any{"query": "snollygoster"})
		var beforeResp searchResp
		parseJSON(t, beforeResult, &beforeResp)
		require.NotEmpty(t, beforeResp.Results, "page must be searchable before deletion")

		callTool(t, c, "delete_page", map[string]any{"page": "Deletable Page"})

		afterResult := callTool(t, c, "search", map[string]any{"query": "snollygoster"})
		var afterResp searchResp
		parseJSON(t, afterResult, &afterResp)

		for _, r := range afterResp.Results {
			assert.NotEqual(t, "Deletable Page", r.Page,
				"deleted page must not appear in search results")
		}
	})
}
