package index_test

import (
	"sort"
	"testing"

	"github.com/edgetools/memento/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFuzzyMatch(t *testing.T) {
	t.Parallel()

	t.Run("exact", func(t *testing.T) {
		t.Parallel()
		ti := index.NewTrigram()
		ti.Add("enchanter")

		matches := ti.FuzzyMatch("enchanter", 0.3)
		assert.Contains(t, matches, "enchanter",
			"exact term should always match itself")
	})

	t.Run("one_char_typo", func(t *testing.T) {
		t.Parallel()
		ti := index.NewTrigram()
		ti.Add("enchanter")
		ti.Add("wizard")
		ti.Add("bard")

		// "enchaner" is "enchanter" with 't' deleted
		// enchanter trigrams: enc,nch,cha,han,ant,nte,ter (7)
		// enchaner  trigrams: enc,nch,cha,han,ane,ner (6)
		// intersection: 4 → Jaccard ≈ 4/9 ≈ 0.44 — well above 0.3
		matches := ti.FuzzyMatch("enchaner", 0.3)
		assert.Contains(t, matches, "enchanter",
			"one-char deletion typo should match the correct term")
		assert.NotContains(t, matches, "wizard")
		assert.NotContains(t, matches, "bard")
	})

	t.Run("two_char_typo", func(t *testing.T) {
		t.Parallel()
		ti := index.NewTrigram()
		ti.Add("enchanter")
		ti.Add("wizard")

		// "enchner" is "enchanter" with 'a' and 't' deleted.
		// Use a permissive threshold (0.15) since a two-char deletion reduces
		// overlap significantly, but the term is still recognisably similar.
		matches := ti.FuzzyMatch("enchner", 0.15)
		assert.Contains(t, matches, "enchanter",
			"two-char deletion typo should still match above a permissive threshold")
		assert.NotContains(t, matches, "wizard")
	})

	t.Run("completely_different", func(t *testing.T) {
		t.Parallel()
		ti := index.NewTrigram()
		ti.Add("enchanter")

		matches := ti.FuzzyMatch("wizard", 0.3)
		assert.NotContains(t, matches, "enchanter",
			"completely different term should not match")
	})

	t.Run("short_term", func(t *testing.T) {
		t.Parallel()
		ti := index.NewTrigram()
		ti.Add("mez")
		ti.Add("mezmerize")

		// "mez" has only one trigram ("mez"). Exact match should still work.
		var matches []string
		assert.NotPanics(t, func() {
			matches = ti.FuzzyMatch("mez", 0.9)
		}, "short term FuzzyMatch should not panic")
		require.NotEmpty(t, matches, "exact short term should match at high threshold")
		assert.Contains(t, matches, "mez")
	})

	t.Run("multiple_matches", func(t *testing.T) {
		t.Parallel()
		ti := index.NewTrigram()
		ti.Add("enchanter")
		ti.Add("enchantment")
		ti.Add("enchanting")
		ti.Add("wizard")
		ti.Add("bard")

		// "enchant" shares high trigram overlap with all three enchant* terms:
		//   enchant trigrams: enc,nch,cha,han,ant (5)
		//   vs enchanter:    enc,nch,cha,han,ant,nte,ter (7)  → Jaccard 5/7 ≈ 0.71
		//   vs enchantment:  enc,nch,cha,han,ant,ntm,tme,men,ent (9) → Jaccard 5/9 ≈ 0.56
		//   vs enchanting:   enc,nch,cha,han,ant,nti,tin,ing (8) → Jaccard 5/8 = 0.625
		matches := ti.FuzzyMatch("enchant", 0.3)
		assert.Contains(t, matches, "enchanter",
			"'enchant' should fuzzy-match 'enchanter'")
		assert.Contains(t, matches, "enchantment",
			"'enchant' should fuzzy-match 'enchantment'")
		assert.Contains(t, matches, "enchanting",
			"'enchant' should fuzzy-match 'enchanting'")
		assert.NotContains(t, matches, "wizard")
		assert.NotContains(t, matches, "bard")
	})

	t.Run("empty_query", func(t *testing.T) {
		t.Parallel()
		ti := index.NewTrigram()
		ti.Add("enchanter")

		var matches []string
		assert.NotPanics(t, func() {
			matches = ti.FuzzyMatch("", 0.3)
		}, "empty query should not panic")
		assert.Empty(t, matches, "empty query should return no matches")
	})

	t.Run("compound_term", func(t *testing.T) {
		t.Parallel()
		ti := index.NewTrigram()
		ti.Add("crowd control")
		ti.Add("enchanter")
		ti.Add("wizard")

		// "crowd contrl" is "crowd control" with 'o' deleted from "control"
		matches := ti.FuzzyMatch("crowd contrl", 0.3)
		assert.Contains(t, matches, "crowd control",
			"typo in compound term should still match the indexed compound term")
	})
}

func TestTrigramAdd(t *testing.T) {
	t.Parallel()

	t.Run("builds_trigrams", func(t *testing.T) {
		t.Parallel()
		// "enchanter" sliding windows of 3: enc,nch,cha,han,ant,nte,ter
		expected := []string{"enc", "nch", "cha", "han", "ant", "nte", "ter"}

		got := index.Trigrams("enchanter")

		sort.Strings(expected)
		sort.Strings(got)
		assert.Equal(t, expected, got,
			"Trigrams('enchanter') should produce the correct 3-char sliding windows")
	})
}

func TestSimilarity(t *testing.T) {
	t.Parallel()

	t.Run("jaccard", func(t *testing.T) {
		t.Parallel()

		// Identical strings: |A∩B|/|A∪B| = |A|/|A| = 1.0
		assert.InDelta(t, 1.0, index.Similarity("enchanter", "enchanter"), 0.001,
			"identical strings should have Jaccard similarity 1.0")

		// Completely disjoint short strings: {abc} ∩ {xyz} = ∅ → 0/2 = 0.0
		assert.InDelta(t, 0.0, index.Similarity("abc", "xyz"), 0.001,
			"completely different short strings should have Jaccard similarity 0.0")

		// Partial overlap:
		//   "abcd" → {abc, bcd}
		//   "bcde" → {bcd, cde}
		//   intersection = {bcd} = 1, union = {abc,bcd,cde} = 3 → 1/3
		assert.InDelta(t, 1.0/3.0, index.Similarity("abcd", "bcde"), 0.001,
			"partial overlap should yield correct Jaccard coefficient (1/3)")

		// One-char deletion:
		//   "enchanter" → enc,nch,cha,han,ant,nte,ter (7 trigrams)
		//   "enchaner"  → enc,nch,cha,han,ane,ner (6 trigrams)
		//   intersection = enc,nch,cha,han (4), union = 7+6-4 = 9 → 4/9
		assert.InDelta(t, 4.0/9.0, index.Similarity("enchanter", "enchaner"), 0.001,
			"one-char deletion typo should yield Jaccard ≈ 0.444 (4/9)")
	})
}
