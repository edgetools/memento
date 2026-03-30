package pages_test

import (
	"strings"
	"testing"

	"github.com/edgetools/memento/pages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("extracts_title_from_h1", func(t *testing.T) {
		t.Parallel()
		p := pages.Parse("crowd control", []byte("# Crowd Control\n\nBody text here."))
		assert.Equal(t, "Crowd Control", p.Title)
	})

	t.Run("no_heading_uses_page_name", func(t *testing.T) {
		t.Parallel()
		p := pages.Parse("crowd control", []byte("Just body text, no heading."))
		assert.Equal(t, "crowd control", p.Title)
	})

	t.Run("extracts_single_wikilink", func(t *testing.T) {
		t.Parallel()
		p := pages.Parse("test", []byte("See [[Enchanter]] for details."))
		assert.Equal(t, []string{"Enchanter"}, p.WikiLinks)
	})

	t.Run("extracts_multiple_wikilinks", func(t *testing.T) {
		t.Parallel()
		p := pages.Parse("test", []byte("[[A]] and [[B]] and [[C]]"))
		assert.ElementsMatch(t, []string{"A", "B", "C"}, p.WikiLinks)
	})

	t.Run("deduplicates_wikilinks", func(t *testing.T) {
		t.Parallel()
		p := pages.Parse("test", []byte("[[A]] is here and also [[A]] again."))
		assert.Equal(t, []string{"A"}, p.WikiLinks)
	})

	t.Run("no_wikilinks", func(t *testing.T) {
		t.Parallel()
		p := pages.Parse("test", []byte("Plain text with no links."))
		assert.Empty(t, p.WikiLinks)
	})

	t.Run("ignores_wikilink_in_code_block", func(t *testing.T) {
		t.Parallel()
		content := "```\n[[Enchanter]] in code block\n```"
		p := pages.Parse("test", []byte(content))
		assert.Empty(t, p.WikiLinks)
	})

	t.Run("ignores_wikilink_in_inline_code", func(t *testing.T) {
		t.Parallel()
		p := pages.Parse("test", []byte("Use `[[Enchanter]]` syntax."))
		assert.Empty(t, p.WikiLinks)
	})

	t.Run("empty_wikilink_ignored", func(t *testing.T) {
		t.Parallel()
		p := pages.Parse("test", []byte("An [[]] empty link."))
		assert.Empty(t, p.WikiLinks)
	})

	t.Run("body_excludes_title", func(t *testing.T) {
		t.Parallel()
		p := pages.Parse("crowd control", []byte("# Crowd Control\n\nBody text here."))
		assert.Equal(t, "Body text here.", p.Body)
	})

	t.Run("h2_not_treated_as_title", func(t *testing.T) {
		t.Parallel()
		p := pages.Parse("crowd control", []byte("## Not Title\n\nSome body."))
		assert.Equal(t, "crowd control", p.Title)
	})

	t.Run("preserves_wikilink_casing", func(t *testing.T) {
		t.Parallel()
		p := pages.Parse("test", []byte("See [[Crowd Control]] for details."))
		require.Len(t, p.WikiLinks, 1)
		assert.Equal(t, "Crowd Control", p.WikiLinks[0])
	})

	t.Run("empty_content", func(t *testing.T) {
		t.Parallel()
		p := pages.Parse("test", []byte(""))
		assert.Equal(t, "test", p.Title)
		assert.Empty(t, p.WikiLinks)
		assert.Empty(t, p.Body)
	})

	t.Run("multiline", func(t *testing.T) {
		t.Parallel()
		content := "# My Page\n\nFirst paragraph with [[Link One]].\n\nSecond paragraph with [[Link Two]] and [[Link One]] again.\n\nThird paragraph."
		p := pages.Parse("my page", []byte(content))
		assert.Equal(t, "My Page", p.Title)
		assert.ElementsMatch(t, []string{"Link One", "Link Two"}, p.WikiLinks)
		assert.Equal(t, strings.Count(content, "\n")+1, p.Lines)
	})
}
