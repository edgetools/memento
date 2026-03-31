package index_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/edgetools/memento/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resultPageNames extracts the Page name from each Result for easy membership assertions.
func resultPageNames(results []index.Result) []string {
	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Page
	}
	return names
}

// findResult returns a pointer to the Result for the given page name, or nil if not found.
func findResult(results []index.Result, page string) *index.Result {
	for i := range results {
		if results[i].Page == page {
			return &results[i]
		}
	}
	return nil
}

func TestIndexSearch(t *testing.T) {
	t.Parallel()

	t.Run("bm25_primary", func(t *testing.T) {
		t.Parallel()
		idx := index.NewIndex()
		idx.Add(makePage("Enchanter", "The enchanter is the primary mez class.", nil))
		idx.Add(makePage("Bard", "A support class with some mez-like songs.", nil))
		idx.Add(makePage("Crowd Control", "Crowd control abilities include mez and root.", nil))

		results := idx.Search("enchanter", 10)

		require.NotEmpty(t, results, "BM25 should return matching pages")
		assert.Equal(t, "Enchanter", results[0].Page,
			"page whose title is the query term should rank first via BM25 title weight")
		// Results must be in descending score order.
		for i := 1; i < len(results); i++ {
			assert.GreaterOrEqualf(t, results[i-1].Score, results[i].Score,
				"results[%d].Score=%f should be >= results[%d].Score=%f",
				i-1, results[i-1].Score, i, results[i].Score)
		}
	})

	t.Run("trigram_fallback_fires", func(t *testing.T) {
		t.Parallel()
		idx := index.NewIndex()
		idx.Add(makePage("Enchanter", "The enchanter specializes in mesmerize spells.", nil))

		// "enchaner" is "enchanter" with 't' deleted — BM25 alone should find 0 results
		// because the stemmed form of "enchaner" does not match "enchant".
		// The trigram fallback must expand the query to rescue this search.
		results := idx.Search("enchaner", 10)

		require.NotEmpty(t, results,
			"one-char-deletion typo should find results via trigram fallback when BM25 returns <3 results")
		assert.Equal(t, "Enchanter", results[0].Page,
			"trigram fallback should surface the page whose title matches the corrected term")
	})

	t.Run("trigram_fallback_skipped", func(t *testing.T) {
		t.Parallel()
		idx := index.NewIndex()
		// Three pages all matching "mez" via BM25 — fallback threshold not triggered.
		idx.Add(makePage("Mez Basics", "Mez is crowd control that stops enemy action completely.", nil))
		idx.Add(makePage("Mez Duration", "Mez duration depends on the caster and target level difference.", nil))
		idx.Add(makePage("Mez Breaking", "Mez breaks immediately on any damage; use it carefully.", nil))
		// An unrelated page — must not appear in results since trigram is skipped.
		idx.Add(makePage("Merchant Guide", "The merchant sells goods and services to adventurers.", nil))

		results := idx.Search("mez", 10)

		require.GreaterOrEqual(t, len(results), 3,
			"BM25 should return at least 3 results, keeping trigram skipped")
		names := resultPageNames(results)
		assert.Contains(t, names, "Mez Basics")
		assert.Contains(t, names, "Mez Duration")
		assert.Contains(t, names, "Mez Breaking")
		assert.NotContains(t, names, "Merchant Guide",
			"unrelated page should not appear; trigram was skipped so no false expansions")
	})

	t.Run("graph_boost_linked", func(t *testing.T) {
		t.Parallel()
		idx := index.NewIndex()
		// "Crowd Control" links to "Enchanter" via wikilink.
		idx.Add(makePage("Crowd Control",
			"Crowd control relies on the [[Enchanter]] for primary mez duties.",
			[]string{"Enchanter"}))
		// "Enchanter" body does NOT mention "mez" directly in a way that matches the query.
		idx.Add(makePage("Enchanter", "A utility class with various spell abilities.", nil))

		// Searching "mez" matches "Crowd Control" directly.
		// "Enchanter" (one hop away) should surface via graph boost.
		results := idx.Search("mez", 10)

		require.NotEmpty(t, results)
		names := resultPageNames(results)
		assert.Contains(t, names, "Crowd Control",
			"directly matching page should appear in results")
		assert.Contains(t, names, "Enchanter",
			"page linked from a direct match should appear in results via graph boost")

		ccResult := findResult(results, "Crowd Control")
		enchResult := findResult(results, "Enchanter")
		require.NotNil(t, ccResult)
		require.NotNil(t, enchResult)
		assert.Greater(t, ccResult.Score, enchResult.Score,
			"direct match should score higher than the graph-boosted linked page")
	})

	t.Run("graph_boost_direct_and_linked", func(t *testing.T) {
		t.Parallel()
		idx := index.NewIndex()
		// "Enchanter" directly mentions "mez" AND is linked from "Crowd Control".
		idx.Add(makePage("Enchanter", "The enchanter mez class handles crowd control.", nil))
		idx.Add(makePage("Crowd Control",
			"Crowd control is managed by the [[Enchanter]] during a pull.",
			[]string{"Enchanter"}))
		idx.Add(makePage("Pulling", "Pulling strategy when no mez class is available.", nil))

		// "Enchanter" is both a direct BM25 match for "enchanter" AND boosted
		// because "Crowd Control" (another direct match) links to it.
		results := idx.Search("enchanter mez", 10)

		require.NotEmpty(t, results)
		enchResult := findResult(results, "Enchanter")
		require.NotNil(t, enchResult, "Enchanter should appear in results")

		// Enchanter has the highest signal: title match (10x) + direct body match + graph boost.
		assert.Equal(t, "Enchanter", results[0].Page,
			"page that is both a direct match and a graph-connected page should rank highest")
	})

	t.Run("relevance_threshold", func(t *testing.T) {
		t.Parallel()
		idx := index.NewIndex()
		// Strong match: page title IS "Enchanter" (10x weight) + body mentions it multiple times.
		idx.Add(makePage("Enchanter",
			"Enchanter is the primary mez class. The enchanter handles crowd control.",
			nil))
		// Weak match: "enchanter" appears exactly once in a very long body — length normalization
		// will penalize this heavily, pushing it well below the 50% relevance threshold.
		longBody := strings.Repeat(
			"This section discusses many unrelated topics at length in great detail. ", 40,
		) + "An enchanter is briefly mentioned here once only."
		idx.Add(makePage("Long Reference", longBody, nil))

		results := idx.Search("enchanter", 10)

		require.NotEmpty(t, results)
		names := resultPageNames(results)
		assert.Contains(t, names, "Enchanter",
			"strong match should always survive the relevance threshold")
		assert.NotContains(t, names, "Long Reference",
			"weak match scoring below 50%% of the top result should be filtered out")
	})

	t.Run("max_results", func(t *testing.T) {
		t.Parallel()
		idx := index.NewIndex()
		for i := 0; i < 8; i++ {
			idx.Add(makePage(
				fmt.Sprintf("Enchanter Page %d", i),
				"The enchanter class handles mez and crowd control duties.",
				nil,
			))
		}

		results := idx.Search("enchanter", 5)
		assert.LessOrEqual(t, len(results), 5,
			"Search should honor the max results limit")
	})

	t.Run("snippet_direct_match", func(t *testing.T) {
		t.Parallel()
		idx := index.NewIndex()
		body := "Introduction to crowd control tactics.\n" +
			"The enchanter specializes in mesmerize spells and crowd control.\n" +
			"Other CC classes include bards and chanters."
		idx.Add(makePage("CC Guide", body, nil))

		results := idx.Search("enchanter", 10)

		require.NotEmpty(t, results)
		snippet := results[0].Snippet
		assert.NotEmpty(t, snippet, "snippet should not be empty for a direct match")
		assert.Contains(t, strings.ToLower(snippet), "enchanter",
			"snippet for a direct match should contain the query term in context")
	})

	t.Run("snippet_title_match", func(t *testing.T) {
		t.Parallel()
		idx := index.NewIndex()
		// Page whose title IS the search term — snippet should show the first paragraph.
		firstParagraph := "The enchanter is a utility class specializing in mesmerize spells."
		body := firstParagraph + "\n\n" +
			"Enchanters can be found in most high-end raid configurations."
		idx.Add(makePage("Enchanter", body, nil))

		results := idx.Search("enchanter", 10)

		require.NotEmpty(t, results)
		assert.Equal(t, "Enchanter", results[0].Page,
			"title-matching page should rank first")
		// For a title match, the snippet should come from the first paragraph.
		assert.Contains(t, results[0].Snippet, "utility class",
			"title match snippet should show the first paragraph content")
	})

	t.Run("snippet_linked_page", func(t *testing.T) {
		t.Parallel()
		idx := index.NewIndex()
		// "Crowd Control" has a direct body match for "mez" AND links to "Enchanter".
		idx.Add(makePage("Crowd Control",
			"During a pull, the [[Enchanter]] is assigned to mez duty first.",
			[]string{"Enchanter"}))
		// "Enchanter" has no body match for "mez".
		idx.Add(makePage("Enchanter", "A utility class with various abilities.", nil))

		results := idx.Search("mez", 10)

		// "Enchanter" surfaces via graph boost from "Crowd Control".
		enchResult := findResult(results, "Enchanter")
		require.NotNil(t, enchResult,
			"Enchanter should appear in results via graph boost from Crowd Control")

		// The snippet for a graph-boosted result comes from the referring page's text,
		// showing the [[link]] in context.
		assert.Contains(t, enchResult.Snippet, "[[Enchanter]]",
			"snippet for a graph-boosted result should show the wikilink in the referring page's context")
	})

	t.Run("snippet_length", func(t *testing.T) {
		t.Parallel()
		idx := index.NewIndex()
		body := "The enchanter is a utility class specializing in mesmerize spells.\n\n" +
			"Enchanters use their powerful mez abilities to control multiple enemies at once.\n\n" +
			"The enchanter toolkit includes stun, mez, slow, and charm effects for CC.\n\n" +
			"See also [[Crowd Control]] for a complete list of crowd control abilities."
		idx.Add(makePage("Enchanter", body, []string{"Crowd Control"}))

		results := idx.Search("enchanter", 10)

		require.NotEmpty(t, results)
		snippet := results[0].Snippet
		assert.NotEmpty(t, snippet, "snippet should not be empty")
		// Design target is ~250 chars; allow a reasonable implementation window.
		assert.LessOrEqual(t, len(snippet), 400,
			"snippet should not far exceed the ~250-char design target")
		assert.GreaterOrEqual(t, len(snippet), 30,
			"snippet should contain meaningful content")
	})

	t.Run("empty_index", func(t *testing.T) {
		t.Parallel()
		idx := index.NewIndex()

		assert.NotPanics(t, func() {
			results := idx.Search("enchanter", 10)
			assert.Empty(t, results, "empty index should return no results")
		})
	})

	t.Run("line_numbers", func(t *testing.T) {
		t.Parallel()
		idx := index.NewIndex()
		// Construct a page where the matching content is well into the body (not line 1).
		var bodyLines []string
		for i := 0; i < 10; i++ {
			bodyLines = append(bodyLines, fmt.Sprintf("Line %d: introductory content about tactics and strategy.", i+1))
		}
		// Match is at line 11 of the body.
		bodyLines = append(bodyLines, "The enchanter is the primary mez class assigned to crowd control.")
		bodyLines = append(bodyLines, "Additional closing content about strategies and group composition.")
		body := strings.Join(bodyLines, "\n")

		idx.Add(makePage("Enchanter Guide", body, nil))

		results := idx.Search("enchanter", 10)

		require.NotEmpty(t, results)
		assert.Greater(t, results[0].Line, 0,
			"line number should be a positive integer pointing into the page content")
		// The page has len(bodyLines)+1 lines (heading counts as line 1).
		assert.LessOrEqual(t, results[0].Line, len(bodyLines)+1,
			"line number should be within the bounds of the page")
	})
}

func TestIndexBuild(t *testing.T) {
	t.Parallel()

	t.Run("indexes_all", func(t *testing.T) {
		t.Parallel()
		idx := index.NewIndex()
		pages := []struct {
			name string
			term string
		}{
			{"Enchanter", "enchanter"},
			{"Bard", "bard"},
			{"Warrior", "warrior"},
			{"Cleric", "cleric"},
			{"Rogue", "rogue"},
		}
		for _, p := range pages {
			idx.Add(makePage(p.name, fmt.Sprintf("The %s is an adventuring class.", p.term), nil))
		}

		// Every added page should be discoverable via its own unique term.
		for _, p := range pages {
			results := idx.Search(p.term, 1)
			require.NotEmptyf(t, results, "page %q should be searchable after being added to the index", p.name)
			assert.Equalf(t, p.name, results[0].Page,
				"searching for %q should return page %q first", p.term, p.name)
		}
	})
}

func TestIndexUpdate(t *testing.T) {
	t.Parallel()

	t.Run("reflects_changes", func(t *testing.T) {
		t.Parallel()
		idx := index.NewIndex()
		// Initial content about "mesmerize".
		idx.Add(makePage("Enchanter", "The enchanter uses mesmerize to control enemies.", nil))

		// Verify initial content is searchable.
		beforeResults := idx.Search("mesmerize", 10)
		require.NotEmpty(t, beforeResults, "initial content should be indexed")
		assert.Equal(t, "Enchanter", beforeResults[0].Page)

		// Re-add with completely different content — must replace, not merge.
		idx.Add(makePage("Enchanter", "The enchanter now focuses on haste and buff spells.", nil))

		// Old content should no longer match.
		staleResults := idx.Search("mesmerize", 10)
		assert.NotContains(t, resultPageNames(staleResults), "Enchanter",
			"stale content should be removed from the index when the page is re-added")

		// New content should now be searchable.
		freshResults := idx.Search("haste", 10)
		assert.Contains(t, resultPageNames(freshResults), "Enchanter",
			"updated content should be findable in the index immediately after re-add")
	})
}
