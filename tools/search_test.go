package tools_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Response types --------------------------------------------------------

// searchLinkedPageDetail represents a deduplicated linked page entry in the
// top-level linked_page_details array.
type searchLinkedPageDetail struct {
	Page    string `json:"page"`
	Snippet string `json:"snippet"`
	Line    int    `json:"line"`
}

type searchResult struct {
	Page        string   `json:"page"`
	Relevance   float64  `json:"relevance"`
	Snippet     string   `json:"snippet"`
	Line        int      `json:"line"`
	LinkedPages []string `json:"linked_pages"`
}

type searchResp struct {
	Results           []searchResult           `json:"results"`
	LinkedPageDetails []searchLinkedPageDetail `json:"linked_page_details"`
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

// findLinkedPageDetail returns a pointer to the searchLinkedPageDetail for the
// given page name, or nil if not present.
func findLinkedPageDetail(details []searchLinkedPageDetail, page string) *searchLinkedPageDetail {
	for i := range details {
		if details[i].Page == page {
			return &details[i]
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

// linkedPageDetailPages extracts the Page field from each searchLinkedPageDetail.
func linkedPageDetailPages(details []searchLinkedPageDetail) []string {
	names := make([]string, len(details))
	for i, d := range details {
		names[i] = d.Page
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

	t.Run("linked_pages_are_string_names", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "The enchanter specializes in mesmerize and crowd control effects.",
		})
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
			"a result with outbound wikilinks must include linked_pages names")
		assert.Contains(t, ccResult.LinkedPages, "Enchanter",
			"linked_pages must contain the name of the linked page")
	})

	t.Run("linked_page_details_at_top_level", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "The enchanter specializes in mesmerize.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Bard",
			"content": "The bard is a support class with songs.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Group Makeup",
			"content": "A good group has [[Enchanter]] and [[Bard]] for support.",
		})

		result := callTool(t, c, "search", map[string]any{"query": "group makeup"})
		var resp searchResp
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.Results, "search should return results")
		gmResult := findSearchResult(resp.Results, "Group Makeup")
		require.NotNil(t, gmResult, "Group Makeup must appear in results")

		// linked_pages should be string names only.
		assert.Contains(t, gmResult.LinkedPages, "Enchanter")
		assert.Contains(t, gmResult.LinkedPages, "Bard")

		// linked_page_details should be at the top level with snippet and line.
		require.NotEmpty(t, resp.LinkedPageDetails,
			"linked_page_details must be present at the top level")

		enchDetail := findLinkedPageDetail(resp.LinkedPageDetails, "Enchanter")
		require.NotNil(t, enchDetail, "Enchanter must appear in linked_page_details")
		assert.NotEmpty(t, enchDetail.Snippet, "linked page detail must include a snippet")
		assert.Greater(t, enchDetail.Line, 0, "linked page detail must include a positive line number")

		bardDetail := findLinkedPageDetail(resp.LinkedPageDetails, "Bard")
		require.NotNil(t, bardDetail, "Bard must appear in linked_page_details")
		assert.NotEmpty(t, bardDetail.Snippet, "linked page detail must include a snippet")
		assert.Greater(t, bardDetail.Line, 0, "linked page detail must include a positive line number")
	})

	t.Run("linked_page_details_deduplicated", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Create a shared target page whose name does not appear in the query,
		// so it ends up only in linked_page_details (not in results).
		callTool(t, c, "write_page", map[string]any{
			"page":    "Shared Target",
			"content": "Shared target page content.",
		})
		// Two pages that both link to Shared Target and share a unique searchable term.
		callTool(t, c, "write_page", map[string]any{
			"page":    "CC Guide",
			"content": "For crowd control see [[Shared Target]] techniques. Dedup overview.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Group Strategy",
			"content": "Group strategy requires [[Shared Target]] for CC. Dedup overview.",
		})

		result := callTool(t, c, "search", map[string]any{"query": "dedup overview"})
		var resp searchResp
		parseJSON(t, result, &resp)

		// CC Guide and Group Strategy must appear in results; Shared Target must not.
		require.NotNil(t, findSearchResult(resp.Results, "CC Guide"), "CC Guide must appear in results")
		require.NotNil(t, findSearchResult(resp.Results, "Group Strategy"), "Group Strategy must appear in results")
		require.Nil(t, findSearchResult(resp.Results, "Shared Target"), "Shared Target must not appear in results")

		// Shared Target must appear in linked_page_details exactly once (deduplicated).
		count := 0
		for _, d := range resp.LinkedPageDetails {
			if strings.EqualFold(d.Page, "Shared Target") {
				count++
			}
		}
		assert.Equal(t, 1, count,
			"a linked page referenced by multiple results must appear exactly once in linked_page_details")
	})

	t.Run("linked_page_details_excludes_results", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Enchanter will match the search query directly (appears in results).
		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "The enchanter is the primary mez class.",
		})
		// Bard is only linked, never matches the query directly.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Bard",
			"content": "The bard is a support class with songs.",
		})
		// CC Guide links to both Enchanter and Bard, and mentions enchanter.
		callTool(t, c, "write_page", map[string]any{
			"page":    "CC Guide",
			"content": "The enchanter role is key. See [[Enchanter]] and [[Bard]] for details.",
		})

		result := callTool(t, c, "search", map[string]any{"query": "enchanter"})
		var resp searchResp
		parseJSON(t, result, &resp)

		// Enchanter should appear in results (it matches the query).
		enchResult := findSearchResult(resp.Results, "Enchanter")
		require.NotNil(t, enchResult, "Enchanter must appear in results")

		// Enchanter must NOT appear in linked_page_details since it's already a result.
		enchDetail := findLinkedPageDetail(resp.LinkedPageDetails, "Enchanter")
		assert.Nil(t, enchDetail,
			"a page already in results must be excluded from linked_page_details")

		// Bard should appear in linked_page_details since it's linked but not a result.
		bardDetail := findLinkedPageDetail(resp.LinkedPageDetails, "Bard")
		assert.NotNil(t, bardDetail,
			"a linked page that is not in results must appear in linked_page_details")
	})

	t.Run("linked_page_details_excludes_results_case_insensitive", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "The enchanter is a mez class.",
		})
		// Link uses different casing.
		callTool(t, c, "write_page", map[string]any{
			"page":    "CC Guide",
			"content": "The enchanter is key. See [[enchanter]] for details.",
		})

		result := callTool(t, c, "search", map[string]any{"query": "enchanter"})
		var resp searchResp
		parseJSON(t, result, &resp)

		// Enchanter is in results; it must not also appear in linked_page_details
		// regardless of the link casing.
		enchDetail := findLinkedPageDetail(resp.LinkedPageDetails, "Enchanter")
		assert.Nil(t, enchDetail,
			"exclusion of result pages from linked_page_details must be case-insensitive")
	})

	t.Run("linked_page_details_ordered_by_first_encounter", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Create target pages.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Alpha",
			"content": "Alpha page content.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Beta",
			"content": "Beta page content.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Gamma",
			"content": "Gamma page content.",
		})

		// A single high-relevance page that links to Alpha, Beta, and Gamma in that order.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Hub",
			"content": "Hub page links to [[Alpha]] and [[Beta]] and [[Gamma]].",
		})

		result := callTool(t, c, "search", map[string]any{"query": "hub"})
		var resp searchResp
		parseJSON(t, result, &resp)

		hubResult := findSearchResult(resp.Results, "Hub")
		require.NotNil(t, hubResult, "Hub must appear in results")

		// linked_page_details should preserve first-encountered order.
		detailNames := linkedPageDetailPages(resp.LinkedPageDetails)
		require.GreaterOrEqual(t, len(detailNames), 3,
			"all three linked pages should appear in linked_page_details")

		// Find indices and verify ordering.
		alphaIdx, betaIdx, gammaIdx := -1, -1, -1
		for i, n := range detailNames {
			switch n {
			case "Alpha":
				alphaIdx = i
			case "Beta":
				betaIdx = i
			case "Gamma":
				gammaIdx = i
			}
		}
		require.NotEqual(t, -1, alphaIdx, "Alpha must be in linked_page_details")
		require.NotEqual(t, -1, betaIdx, "Beta must be in linked_page_details")
		require.NotEqual(t, -1, gammaIdx, "Gamma must be in linked_page_details")

		assert.Less(t, alphaIdx, betaIdx,
			"Alpha must appear before Beta in linked_page_details (first-encountered order)")
		assert.Less(t, betaIdx, gammaIdx,
			"Beta must appear before Gamma in linked_page_details (first-encountered order)")
	})

	t.Run("linked_page_details_empty_when_no_links", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Standalone",
			"content": "This page has no wikilinks at all.",
		})

		result := callTool(t, c, "search", map[string]any{"query": "standalone"})
		var resp searchResp
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.Results, "search should return results")
		assert.Empty(t, resp.LinkedPageDetails,
			"linked_page_details must be empty when no results have outbound links")
	})

	t.Run("max_tokens_accounts_for_linked_page_details", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Create many linked pages to inflate the linked_page_details section.
		for i := range 10 {
			callTool(t, c, "write_page", map[string]any{
				"page": fmt.Sprintf("Linked Target %d", i),
				"content": fmt.Sprintf("Target %d has a very long body that uses many tokens. "+
					"This content is designed to inflate the token count significantly "+
					"when included as a snippet in linked_page_details.", i),
			})
		}
		// A single result page that links to all targets.
		var sb strings.Builder
		for i := range 10 {
			fmt.Fprintf(&sb, "[[Linked Target %d]] ", i)
		}
		links := sb.String()
		callTool(t, c, "write_page", map[string]any{
			"page":    "Linker",
			"content": "The linker page references " + links,
		})

		// Baseline: no token limit.
		resultFull := callTool(t, c, "search", map[string]any{"query": "linker"})
		var respFull searchResp
		parseJSON(t, resultFull, &respFull)
		require.NotEmpty(t, respFull.Results, "baseline search should return results")

		// With a tight token budget, the total response (including linked_page_details)
		// must fit. This means either fewer results or fewer linked_page_details entries.
		resultLimited := callTool(t, c, "search", map[string]any{
			"query":      "linker",
			"max_tokens": 30,
		})
		var respLimited searchResp
		parseJSON(t, resultLimited, &respLimited)

		// The total response under budget should be smaller: fewer results OR
		// fewer linked_page_details compared to the unlimited baseline.
		fullTotal := len(respFull.Results) + len(respFull.LinkedPageDetails)
		limitedTotal := len(respLimited.Results) + len(respLimited.LinkedPageDetails)
		assert.Less(t, limitedTotal, fullTotal,
			"max_tokens budget must constrain the total response size including linked_page_details")
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
		assert.Empty(t, resp.LinkedPageDetails,
			"linked_page_details must be empty when there are no results")
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

	t.Run("multiple_results_share_linked_page", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// A shared linked target — content must NOT match the query so it stays out of results.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Shared Buff",
			"content": "Shared buff ability that boosts melee damage and defense.",
		})
		// Two result pages both linking to the same target, both matching "xyzsharedlink".
		callTool(t, c, "write_page", map[string]any{
			"page":    "Support Guide A",
			"content": "xyzsharedlink strategies using [[Shared Buff]] for groups.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Support Guide B",
			"content": "More xyzsharedlink strategies with [[Shared Buff]] buffs.",
		})

		result := callTool(t, c, "search", map[string]any{"query": "xyzsharedlink"})
		var resp searchResp
		parseJSON(t, result, &resp)

		// Both guides should list Shared Buff in their linked_pages as a string name.
		// Shared Buff must not appear in results (its content does not match the query).
		guideA := findSearchResult(resp.Results, "Support Guide A")
		guideB := findSearchResult(resp.Results, "Support Guide B")

		require.NotNil(t, guideA, "Support Guide A must appear in results")
		require.NotNil(t, guideB, "Support Guide B must appear in results")
		require.Nil(t, findSearchResult(resp.Results, "Shared Buff"),
			"Shared Buff must not appear in results so it can be verified in linked_page_details")

		assert.Contains(t, guideA.LinkedPages, "Shared Buff",
			"Support Guide A must reference Shared Buff in linked_pages")
		assert.Contains(t, guideB.LinkedPages, "Shared Buff",
			"Support Guide B must reference Shared Buff in linked_pages")

		// Shared Buff should appear exactly once in linked_page_details.
		count := 0
		for _, d := range resp.LinkedPageDetails {
			if d.Page == "Shared Buff" {
				count++
			}
		}
		assert.Equal(t, 1, count,
			"a linked page referenced by multiple results must appear exactly once in linked_page_details")
	})

	t.Run("broken_link_excluded_from_linked_pages", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// A page that links to a non-existent page.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Broken Link Page",
			"content": "See [[Nonexistent Page]] for broken link details.",
		})

		result := callTool(t, c, "search", map[string]any{"query": "broken link"})
		var resp searchResp
		parseJSON(t, result, &resp)

		blResult := findSearchResult(resp.Results, "Broken Link Page")
		require.NotNil(t, blResult, "Broken Link Page must appear in results")
		assert.NotContains(t, blResult.LinkedPages, "Nonexistent Page",
			"broken links (to non-existent pages) must not appear in linked_pages")

		// Also must not appear in linked_page_details.
		nonexDetail := findLinkedPageDetail(resp.LinkedPageDetails, "Nonexistent Page")
		assert.Nil(t, nonexDetail,
			"broken links must not appear in linked_page_details")
	})
}
