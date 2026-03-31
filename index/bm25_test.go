package index_test

import (
	"strings"
	"testing"

	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makePage constructs a pages.Page without touching the filesystem.
// Shared across all index_test files (bm25_test.go, trigram_test.go, graph_test.go, index_test.go).
func makePage(name, body string, links []string) pages.Page {
	return pages.Page{
		Name:      name,
		Title:     name,
		Body:      body,
		WikiLinks: links,
		Lines:     len(strings.Split(body, "\n")),
	}
}

// pageNames extracts the Name field from a slice of SearchResults for easy assertions.
func pageNames(results []index.SearchResult) []string {
	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Name
	}
	return names
}

func TestBM25Search(t *testing.T) {
	t.Parallel()

	t.Run("title_match_ranks_highest", func(t *testing.T) {
		t.Parallel()
		b := index.NewBM25()
		// Page whose title IS "Enchanter" — strongest possible signal
		b.Add(makePage("Enchanter", "A utility class with mez spells.", nil))
		// Page that merely mentions "enchanter" in body
		b.Add(makePage("Crowd Control", "The enchanter is the primary crowd control class.", nil))

		results := b.Search("enchanter", 10)
		require.NotEmpty(t, results)
		require.GreaterOrEqual(t, len(results), 2)
		assert.Equal(t, "Enchanter", results[0].Name,
			"title-matching page should rank first")
		assert.Greater(t, results[0].Score, results[1].Score,
			"title match should score higher than body mention")
	})

	t.Run("wikilink_outranks_body", func(t *testing.T) {
		t.Parallel()
		b := index.NewBM25()
		// Page with [[Enchanter]] as a wikilink — explicit relationship declaration (3x weight)
		b.Add(makePage("Crowd Control", "CC refers to abilities limiting enemy actions.", []string{"Enchanter"}))
		// Page with "enchanter" only in body (no wikilink)
		b.Add(makePage("Party Roles", "The enchanter handles mez duties in a group.", nil))

		results := b.Search("enchanter", 10)
		require.GreaterOrEqual(t, len(results), 2)
		assert.Equal(t, "Crowd Control", results[0].Name,
			"wikilink page should rank above body-only mention")
		assert.Greater(t, results[0].Score, results[1].Score,
			"wikilink (3x weight) should outscore body mention (1x weight)")
	})

	t.Run("body_match_works", func(t *testing.T) {
		t.Parallel()
		b := index.NewBM25()
		b.Add(makePage("Strategy", "Use crowd control to manage multiple enemies at once.", nil))

		results := b.Search("crowd", 10)
		require.NotEmpty(t, results)
		assert.Equal(t, "Strategy", results[0].Name)
	})

	t.Run("multi_term_query", func(t *testing.T) {
		t.Parallel()
		b := index.NewBM25()
		// Page mentioning both terms
		b.Add(makePage("Crowd Control", "Crowd control limits enemy movement and actions.", nil))
		// Pages mentioning only one term each
		b.Add(makePage("Crowd Size", "Large crowds require special management.", nil))
		b.Add(makePage("Control Mechanics", "Control abilities have diminishing returns.", nil))

		results := b.Search("crowd control", 10)
		require.NotEmpty(t, results)
		assert.Equal(t, "Crowd Control", results[0].Name,
			"page containing both query terms should rank highest")
	})

	t.Run("stemming", func(t *testing.T) {
		t.Parallel()
		b := index.NewBM25()
		b.Add(makePage("Enchanter", "The enchanter uses enchantment spells.", nil))

		// "enchanting" should stem to the same root as "enchant(er)" via Porter stemming
		results := b.Search("enchanting", 10)
		require.NotEmpty(t, results, "Porter stemming should allow 'enchanting' to match 'enchanter'")
		assert.Equal(t, "Enchanter", results[0].Name)
	})

	t.Run("stop_words_ignored", func(t *testing.T) {
		t.Parallel()
		b := index.NewBM25()
		b.Add(makePage("Enchanter", "A class specializing in mez.", nil))

		// "the" is a stop word — filtering it should leave just "enchanter"
		withStop := b.Search("the enchanter", 10)
		withoutStop := b.Search("enchanter", 10)

		require.NotEmpty(t, withStop)
		require.NotEmpty(t, withoutStop)
		assert.Equal(t, "Enchanter", withStop[0].Name,
			"stop word 'the' should not prevent the result from appearing")
		assert.InDelta(t, withoutStop[0].Score, withStop[0].Score, 0.001,
			"stop words should not affect relevance scoring")
	})

	t.Run("no_results", func(t *testing.T) {
		t.Parallel()
		b := index.NewBM25()
		b.Add(makePage("Enchanter", "Specializes in mesmerize.", nil))

		results := b.Search("xyzzy", 10)
		assert.Empty(t, results, "absent term should return no results")
	})

	t.Run("length_normalization", func(t *testing.T) {
		t.Parallel()
		b := index.NewBM25()
		// Short page: "enchanter" is dense relative to page length
		b.Add(makePage("Short Page", "enchanter enchanter enchanter.", nil))
		// Long page: "enchanter" appears once buried in irrelevant content
		longBody := strings.Repeat("some irrelevant filler content about many other topics in detail. ", 50) +
			"enchanter"
		b.Add(makePage("Long Page", longBody, nil))

		results := b.Search("enchanter", 10)
		require.GreaterOrEqual(t, len(results), 2)
		assert.Equal(t, "Short Page", results[0].Name,
			"BM25 length normalization: short dense page should outscore long sparse page")
	})

	t.Run("case_insensitive", func(t *testing.T) {
		t.Parallel()
		b := index.NewBM25()
		b.Add(makePage("Enchanter", "Specializes in mez.", nil))

		results := b.Search("ENCHANTER", 10)
		require.NotEmpty(t, results, "query should be case-insensitive")
		assert.Equal(t, "Enchanter", results[0].Name)
	})

	t.Run("compound_wikilink", func(t *testing.T) {
		t.Parallel()
		b := index.NewBM25()
		// Page with [[Crowd Control]] compound wikilink — indexed as phrase AND individual terms
		b.Add(makePage("Dungeon Strategy",
			"Always assign [[Crowd Control]] duties before pulling.",
			[]string{"Crowd Control"}))
		// Page with "crowd" and "control" appearing separately in body
		b.Add(makePage("Notes", "The crowd was under control during the pull.", nil))

		// Compound wikilink should give "Dungeon Strategy" a phrase-level boost
		results := b.Search("crowd control", 10)
		require.NotEmpty(t, results)
		assert.Equal(t, "Dungeon Strategy", results[0].Name,
			"compound wikilink [[Crowd Control]] should outrank separate body mentions")
	})

	t.Run("empty_query", func(t *testing.T) {
		t.Parallel()
		b := index.NewBM25()
		b.Add(makePage("Enchanter", "A mez class.", nil))

		assert.NotPanics(t, func() {
			results := b.Search("", 10)
			assert.Empty(t, results, "empty query should return no results")
		})
	})

	t.Run("multiple_pages_ranked", func(t *testing.T) {
		t.Parallel()
		b := index.NewBM25()
		// Six pages with varying relevance to "enchanter"
		b.Add(makePage("Enchanter", "The enchanter is a crowd control class.", nil))
		b.Add(makePage("Bard", "Bard also has some enchanter-like abilities.", nil))
		b.Add(makePage("Crowd Control", "Crowd control is handled by the [[Enchanter]].", []string{"Enchanter"}))
		b.Add(makePage("Party Composition", "A good party includes an enchanter for mez.", nil))
		b.Add(makePage("Pulling", "Pull with care when no enchanter is present.", nil))
		b.Add(makePage("Mez", "Mesmerize spells like those the enchanter casts.", nil))

		results := b.Search("enchanter", 10)
		require.GreaterOrEqual(t, len(results), 5, "should return 5+ relevant pages")

		// Results must be in descending order by score
		for i := 1; i < len(results); i++ {
			assert.GreaterOrEqualf(t, results[i-1].Score, results[i].Score,
				"results not in descending order: results[%d].Score=%f < results[%d].Score=%f",
				i-1, results[i-1].Score, i, results[i].Score)
		}
	})
}

func TestAddRemove(t *testing.T) {
	t.Parallel()

	t.Run("add_then_search", func(t *testing.T) {
		t.Parallel()
		b := index.NewBM25()
		b.Add(makePage("Enchanter", "A utility class specializing in mez.", nil))

		results := b.Search("enchanter", 10)
		require.NotEmpty(t, results, "added page should be findable immediately")
		assert.Equal(t, "Enchanter", results[0].Name)
	})

	t.Run("remove_then_search", func(t *testing.T) {
		t.Parallel()
		b := index.NewBM25()
		b.Add(makePage("Enchanter", "A utility class specializing in mez.", nil))
		b.Remove("Enchanter")

		results := b.Search("enchanter", 10)
		assert.Empty(t, results, "removed page should not appear in search results")
	})

	t.Run("update_page", func(t *testing.T) {
		t.Parallel()
		b := index.NewBM25()
		// Initial content about mez
		b.Add(makePage("Enchanter", "A utility class specializing in mez.", nil))
		// Re-add with completely different content — update, not duplicate
		b.Add(makePage("Enchanter", "A support class specializing in songs and buffs.", nil))

		// Old content should no longer be indexed
		mezResults := b.Search("mez", 10)
		assert.NotContains(t, pageNames(mezResults), "Enchanter",
			"stale content should be replaced on re-add")

		// New content should now be findable
		songResults := b.Search("songs", 10)
		assert.Contains(t, pageNames(songResults), "Enchanter",
			"updated content should be indexed")
	})
}
