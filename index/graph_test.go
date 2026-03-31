package index_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/edgetools/memento/index"
	"github.com/stretchr/testify/assert"
)

// sortStrings returns a sorted copy of s — used for order-independent equality checks.
func sortStrings(s []string) []string {
	cp := make([]string, len(s))
	copy(cp, s)
	sort.Strings(cp)
	return cp
}

// containsCaseInsensitive reports whether slice contains target, ignoring case.
func containsCaseInsensitive(slice []string, target string) bool {
	target = strings.ToLower(target)
	for _, s := range slice {
		if strings.ToLower(s) == target {
			return true
		}
	}
	return false
}

func TestGraph(t *testing.T) {
	t.Parallel()

	t.Run("LinksTo/outbound", func(t *testing.T) {
		t.Parallel()
		g := index.NewGraph()
		g.Add(makePage("A", "body text", []string{"B", "C"}))

		links := sortStrings(g.LinksTo("A"))
		assert.Equal(t, []string{"B", "C"}, links,
			"LinksTo should return all outbound link targets for the page")
	})

	t.Run("LinkedFrom/inbound", func(t *testing.T) {
		t.Parallel()
		g := index.NewGraph()
		g.Add(makePage("A", "body text", []string{"B"}))

		refs := g.LinkedFrom("B")
		assert.Len(t, refs, 1, "B should have exactly one inbound link")
		assert.Contains(t, refs, "A", "LinkedFrom B should contain A")
	})

	t.Run("Bidirectional/symmetry", func(t *testing.T) {
		t.Parallel()
		g := index.NewGraph()
		g.Add(makePage("Crowd Control", "A CC class uses [[Enchanter]].", []string{"Enchanter"}))

		// Outbound: Crowd Control links to Enchanter
		assert.Contains(t, g.LinksTo("Crowd Control"), "Enchanter",
			"LinksTo should reflect outbound wikilinks from the page")

		// Inbound: Enchanter is linked from Crowd Control
		assert.Contains(t, g.LinkedFrom("Enchanter"), "Crowd Control",
			"LinkedFrom should reflect the reverse direction of every outbound link")
	})

	t.Run("Remove/cleans_both_directions", func(t *testing.T) {
		t.Parallel()
		g := index.NewGraph()
		g.Add(makePage("A", "body", []string{"B", "C"}))
		g.Add(makePage("B", "body", nil))
		g.Add(makePage("C", "body", nil))

		g.Remove("A")

		assert.Empty(t, g.LinksTo("A"),
			"LinksTo for a removed page should be empty")
		assert.NotContains(t, g.LinkedFrom("B"), "A",
			"LinkedFrom B should not contain the removed page A")
		assert.NotContains(t, g.LinkedFrom("C"), "A",
			"LinkedFrom C should not contain the removed page A")
	})

	t.Run("Add/deduplicates", func(t *testing.T) {
		t.Parallel()
		g := index.NewGraph()
		// WikiLinks slice lists the same target twice — should be stored only once.
		g.Add(makePage("A", "body", []string{"B", "B"}))

		links := g.LinksTo("A")
		count := 0
		for _, l := range links {
			if l == "B" {
				count++
			}
		}
		assert.Equal(t, 1, count,
			"duplicate wikilink targets should be deduplicated in the graph")
	})

	t.Run("LinksTo/nonexistent", func(t *testing.T) {
		t.Parallel()
		g := index.NewGraph()

		assert.NotPanics(t, func() {
			links := g.LinksTo("doesnotexist")
			assert.Empty(t, links, "LinksTo for an unknown page should return empty, not panic")
		})
	})

	t.Run("LinkedFrom/nonexistent", func(t *testing.T) {
		t.Parallel()
		g := index.NewGraph()

		assert.NotPanics(t, func() {
			refs := g.LinkedFrom("doesnotexist")
			assert.Empty(t, refs, "LinkedFrom for an unknown page should return empty, not panic")
		})
	})

	t.Run("Update/replaces_links", func(t *testing.T) {
		t.Parallel()
		g := index.NewGraph()
		g.Add(makePage("A", "body", []string{"B", "C"}))

		// Re-add A with completely different outbound links — must replace, not accumulate.
		g.Add(makePage("A", "updated body", []string{"D"}))

		links := g.LinksTo("A")
		assert.Contains(t, links, "D", "updated links should contain the new target D")
		assert.NotContains(t, links, "B", "old link B should be removed after update")
		assert.NotContains(t, links, "C", "old link C should be removed after update")

		assert.NotContains(t, g.LinkedFrom("B"), "A",
			"LinkedFrom B should be cleared after A's links are replaced")
		assert.NotContains(t, g.LinkedFrom("C"), "A",
			"LinkedFrom C should be cleared after A's links are replaced")
		assert.Contains(t, g.LinkedFrom("D"), "A",
			"new target D should now list A as a referrer")
	})

	t.Run("Remove/nonexistent_target", func(t *testing.T) {
		t.Parallel()
		g := index.NewGraph()
		// A links to "X", which has no corresponding page entry in the graph.
		g.Add(makePage("A", "body", []string{"X"}))

		assert.NotPanics(t, func() {
			g.Remove("A")
		}, "removing a page whose link targets have no graph entry should not panic")

		assert.Empty(t, g.LinksTo("A"),
			"LinksTo should be empty after the page is removed")
	})

	t.Run("Graph/case_insensitive", func(t *testing.T) {
		t.Parallel()
		g := index.NewGraph()
		g.Add(makePage("Page Alpha", "body", []string{"Page Beta"}))

		// LinksTo with non-canonical casing of the source name.
		fromLower := g.LinksTo("page alpha")
		fromUpper := g.LinksTo("PAGE ALPHA")
		canonical := g.LinksTo("Page Alpha")

		assert.NotEmpty(t, fromLower,
			"LinksTo should work with lowercase source page name")
		assert.NotEmpty(t, fromUpper,
			"LinksTo should work with uppercase source page name")
		assert.Equal(t, len(canonical), len(fromLower),
			"case variants should return the same number of links as the canonical form")

		// LinkedFrom with non-canonical casing of the target name.
		refsLower := g.LinkedFrom("page beta")
		refsUpper := g.LinkedFrom("PAGE BETA")
		assert.NotEmpty(t, refsLower,
			"LinkedFrom should work with lowercase target page name")
		assert.NotEmpty(t, refsUpper,
			"LinkedFrom should work with uppercase target page name")

		// Verify the returned referrer is recognisably "Page Alpha" regardless of case.
		assert.True(t, containsCaseInsensitive(refsLower, "Page Alpha"),
			"referrer name should be found case-insensitively in LinkedFrom result")
	})
}
