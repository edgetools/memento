package tools_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type renamePageResp struct {
	Page    string `json:"page"`
	OldName string `json:"old_name"`
}

// ---- TestRenamePage --------------------------------------------------------

func TestRenamePage(t *testing.T) {
	t.Parallel()

	t.Run("renames", func(t *testing.T) {
		t.Parallel()
		c, store, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "Crowd control abilities limit enemy actions.",
		})

		callTool(t, c, "rename_page", map[string]any{
			"page":     "Crowd Control",
			"new_name": "Crowd Control Mechanics",
		})

		assert.True(t, store.Exists("Crowd Control Mechanics"),
			"new page name should exist in the store after rename")
		assert.False(t, store.Exists("Crowd Control"),
			"old page name should not exist in the store after rename")

		// get_page should succeed for the new name and fail for the old name.
		callTool(t, c, "get_page", map[string]any{"page": "Crowd Control Mechanics"})
		callToolExpectError(t, c, "get_page", map[string]any{"page": "Crowd Control"})
	})

	t.Run("updates_heading", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "CC basics.",
		})

		callTool(t, c, "rename_page", map[string]any{
			"page":     "Crowd Control",
			"new_name": "Crowd Control Mechanics",
		})

		resp := getPageFull(t, c, "Crowd Control Mechanics")
		assert.True(t, strings.HasPrefix(resp.Content, "# Crowd Control Mechanics"),
			"renamed page must have its heading updated to the new name")
		assert.NotContains(t, resp.Content, "# Crowd Control\n",
			"old heading must be replaced by the new heading")
	})

	t.Run("updates_wikilinks", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "Uses [[Crowd Control]] abilities in combat.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "CC basics.",
		})

		callTool(t, c, "rename_page", map[string]any{
			"page":     "Crowd Control",
			"new_name": "Crowd Control Mechanics",
		})

		resp := getPageFull(t, c, "Enchanter")
		assert.Contains(t, resp.Content, "[[Crowd Control Mechanics]]",
			"wikilink must be updated to the new page name")
		assert.NotContains(t, resp.Content, "[[Crowd Control]]",
			"old wikilink must be replaced")
	})

	t.Run("case_insensitive_link_update", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// The wikilink uses all-lowercase casing, different from the page's canonical name.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "Primary ability is [[crowd control]] techniques.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "CC basics.",
		})

		callTool(t, c, "rename_page", map[string]any{
			"page":     "Crowd Control",
			"new_name": "Crowd Control Mechanics",
		})

		resp := getPageFull(t, c, "Enchanter")
		assert.NotContains(t, resp.Content, "[[crowd control]]",
			"lowercase wikilink to renamed page must be updated")
		assert.Contains(t, resp.Content, "[[Crowd Control Mechanics]]",
			"updated wikilink must use the new page name")
	})

	t.Run("preserves_surrounding_text", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "Primary role: [[Crowd Control]] expert. Secondary: support class.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "CC basics.",
		})

		callTool(t, c, "rename_page", map[string]any{
			"page":     "Crowd Control",
			"new_name": "Crowd Control Mechanics",
		})

		resp := getPageFull(t, c, "Enchanter")
		// Text surrounding the updated link must be intact.
		assert.Contains(t, resp.Content, "Primary role:")
		assert.Contains(t, resp.Content, "expert. Secondary: support class.")
		assert.Contains(t, resp.Content, "[[Crowd Control Mechanics]]")
	})

	t.Run("multiple_pages_updated", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Page A",
			"content": "Mentions [[Target Page]] as a reference.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Page B",
			"content": "Also uses [[Target Page]] here.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Page C",
			"content": "The [[Target Page]] concept is covered.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Target Page",
			"content": "The target concept.",
		})

		callTool(t, c, "rename_page", map[string]any{
			"page":     "Target Page",
			"new_name": "Target Page Renamed",
		})

		for _, pageName := range []string{"Page A", "Page B", "Page C"} {
			resp := getPageFull(t, c, pageName)
			assert.Contains(t, resp.Content, "[[Target Page Renamed]]",
				"page %q should have its wikilink updated to the new name", pageName)
			assert.NotContains(t, resp.Content, "[[Target Page]]",
				"page %q should no longer contain the old wikilink", pageName)
		}
	})

	t.Run("self_referential_link", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// The page links to itself via wikilink.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Ouroboros",
			"content": "The [[Ouroboros]] concept refers to itself recursively.",
		})

		callTool(t, c, "rename_page", map[string]any{
			"page":     "Ouroboros",
			"new_name": "Self Reference",
		})

		resp := getPageFull(t, c, "Self Reference")
		assert.Contains(t, resp.Content, "[[Self Reference]]",
			"self-referential link must be updated to the new page name")
		assert.NotContains(t, resp.Content, "[[Ouroboros]]",
			"old self-referential link must be replaced")
	})

	t.Run("target_exists_errors", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Old Name",
			"content": "Content to rename.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "New Name",
			"content": "Already existing page.",
		})

		callToolExpectError(t, c, "rename_page", map[string]any{
			"page":     "Old Name",
			"new_name": "New Name",
		})
	})

	t.Run("source_not_found_errors", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callToolExpectError(t, c, "rename_page", map[string]any{
			"page":     "This Page Does Not Exist",
			"new_name": "Something Else",
		})
	})

	t.Run("returns_both_names", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "CC basics.",
		})

		result := callTool(t, c, "rename_page", map[string]any{
			"page":     "Crowd Control",
			"new_name": "Crowd Control Mechanics",
		})

		var resp renamePageResp
		parseJSON(t, result, &resp)
		assert.Equal(t, "Crowd Control Mechanics", resp.Page,
			"response.page must be the new name")
		assert.Equal(t, "Crowd Control", resp.OldName,
			"response.old_name must be the previous name")
	})

	t.Run("updates_index", func(t *testing.T) {
		t.Parallel()
		c, _, idx := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "The enchanter class handles crowd control.",
		})

		callTool(t, c, "rename_page", map[string]any{
			"page":     "Enchanter",
			"new_name": "Enchanter Expert",
		})

		// Old name must no longer be findable.
		oldResults := idx.Search("enchanter", 10)
		for _, r := range oldResults {
			assert.NotEqual(t, "Enchanter", r.Page,
				"old name should not appear in index after rename")
		}

		// New name must be immediately searchable.
		newResults := idx.Search("enchanter expert", 10)
		require.NotEmpty(t, newResults, "renamed page must be searchable by new name")
		assert.Equal(t, "Enchanter Expert", newResults[0].Page)
	})

	t.Run("case_insensitive_source", func(t *testing.T) {
		t.Parallel()
		c, store, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "CC basics.",
		})

		// Rename using lowercase — must find the page case-insensitively.
		callTool(t, c, "rename_page", map[string]any{
			"page":     "crowd control",
			"new_name": "Crowd Control Mechanics",
		})

		assert.True(t, store.Exists("Crowd Control Mechanics"),
			"rename with lowercase source should succeed via case-insensitive lookup")
		assert.False(t, store.Exists("Crowd Control"),
			"original page must no longer exist after rename")
	})
}
