package tools_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPatchPageCreateOnWrite covers the create-on-write behaviour for append and
// prepend operations on pages that do not yet exist, as specified in the change
// request "patch_page Create-on-Write for Append/Prepend".
func TestPatchPageCreateOnWrite(t *testing.T) {
	t.Parallel()

	// append on a nonexistent page must create the page with an auto-generated
	// heading and the appended content.
	t.Run("append_creates_page", func(t *testing.T) {
		t.Parallel()
		c, store, _ := setupTestServer(t)

		result := callTool(t, c, "patch_page", map[string]any{
			"page": "Aggro Mechanics",
			"operations": []map[string]any{
				{
					"op":      "append",
					"content": "New note about aggro.",
				},
			},
		})

		require.False(t, result.IsError, "append on a nonexistent page should succeed (create-on-write)")
		assert.True(t, store.Exists("Aggro Mechanics"), "page must exist in the store after auto-creation via append")

		resp := getPageFull(t, c, "Aggro Mechanics")
		assert.Equal(t, "Aggro Mechanics", resp.Page)
		assert.Contains(t, resp.Content, "New note about aggro.")
	})

	// prepend on a nonexistent page must create the page with an auto-generated
	// heading and the prepended content inserted after the heading.
	t.Run("prepend_creates_page", func(t *testing.T) {
		t.Parallel()
		c, store, _ := setupTestServer(t)

		result := callTool(t, c, "patch_page", map[string]any{
			"page": "Tank Roles",
			"operations": []map[string]any{
				{
					"op":      "prepend",
					"content": "Introductory tank note.\n\n",
				},
			},
		})

		require.False(t, result.IsError, "prepend on a nonexistent page should succeed (create-on-write)")
		assert.True(t, store.Exists("Tank Roles"), "page must exist in the store after auto-creation via prepend")

		resp := getPageFull(t, c, "Tank Roles")
		assert.Equal(t, "Tank Roles", resp.Page)
		assert.Contains(t, resp.Content, "Introductory tank note.")
	})

	// The auto-created page must have the managed H1 heading, just as write_page
	// produces, and the heading must appear at the very top of the content.
	t.Run("append_creates_page_with_heading", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		result := callTool(t, c, "patch_page", map[string]any{
			"page": "Healer Guide",
			"operations": []map[string]any{
				{
					"op":      "append",
					"content": "Healers keep the party alive.",
				},
			},
		})
		require.False(t, result.IsError, "append on a nonexistent page should succeed (create-on-write)")

		resp := getPageFull(t, c, "Healer Guide")
		assert.True(t, strings.HasPrefix(resp.Content, "# Healer Guide"),
			"auto-created page must start with the correct H1 heading")
	})

	// prepend on a new page: heading must remain at the top; prepended content
	// appears immediately after the heading.
	t.Run("prepend_creates_page_with_heading", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		result := callTool(t, c, "patch_page", map[string]any{
			"page": "Bard Guide",
			"operations": []map[string]any{
				{
					"op":      "prepend",
					"content": "Bards provide group support.\n\n",
				},
			},
		})
		require.False(t, result.IsError, "prepend on a nonexistent page should succeed (create-on-write)")

		resp := getPageFull(t, c, "Bard Guide")
		assert.True(t, strings.HasPrefix(resp.Content, "# Bard Guide"),
			"auto-created page must start with the correct H1 heading after prepend")
		assert.Contains(t, resp.Content, "Bards provide group support.")

		// The heading must come before the prepended content.
		headingIdx := strings.Index(resp.Content, "# Bard Guide")
		contentIdx := strings.Index(resp.Content, "Bards provide group support.")
		assert.Less(t, headingIdx, contentIdx,
			"heading must appear before the prepended content on a newly created page")
	})

	// replace on a nonexistent page must still return an error (existence-dependent).
	t.Run("replace_nonexistent_errors", func(t *testing.T) {
		t.Parallel()
		c, store, _ := setupTestServer(t)

		callToolExpectError(t, c, "patch_page", map[string]any{
			"page": "Ghost Page",
			"operations": []map[string]any{
				{
					"op":  "replace",
					"old": "text that cannot exist",
					"new": "replacement",
				},
			},
		})

		assert.False(t, store.Exists("Ghost Page"),
			"replace on a nonexistent page must not create the page")
	})

	// replace_lines on a nonexistent page must still return an error
	// (existence-dependent).
	t.Run("replace_lines_nonexistent_errors", func(t *testing.T) {
		t.Parallel()
		c, store, _ := setupTestServer(t)

		callToolExpectError(t, c, "patch_page", map[string]any{
			"page": "Phantom Page",
			"operations": []map[string]any{
				{
					"op":    "replace_lines",
					"lines": "1-2",
					"new":   "replacement",
				},
			},
		})

		assert.False(t, store.Exists("Phantom Page"),
			"replace_lines on a nonexistent page must not create the page")
	})

	// Mixed append + replace on a nonexistent page must fail atomically:
	// replace cannot be applied (no existing content), so the whole call is
	// rejected and the page must not be created.
	t.Run("mixed_append_replace_nonexistent_errors", func(t *testing.T) {
		t.Parallel()
		c, store, _ := setupTestServer(t)

		callToolExpectError(t, c, "patch_page", map[string]any{
			"page": "Mixed Phantom",
			"operations": []map[string]any{
				{
					"op":      "append",
					"content": "This would be fine on its own.",
				},
				{
					"op":  "replace",
					"old": "text that cannot exist",
					"new": "replacement",
				},
			},
		})

		assert.False(t, store.Exists("Mixed Phantom"),
			"mixed append+replace on a nonexistent page must not partially create the page (atomicity)")
	})

	// Mixed prepend + replace_lines on a nonexistent page must also fail
	// atomically.
	t.Run("mixed_prepend_replace_lines_nonexistent_errors", func(t *testing.T) {
		t.Parallel()
		c, store, _ := setupTestServer(t)

		callToolExpectError(t, c, "patch_page", map[string]any{
			"page": "Another Phantom",
			"operations": []map[string]any{
				{
					"op":      "prepend",
					"content": "Would be created.",
				},
				{
					"op":    "replace_lines",
					"lines": "2-3",
					"new":   "replacement",
				},
			},
		})

		assert.False(t, store.Exists("Another Phantom"),
			"mixed prepend+replace_lines on a nonexistent page must not partially create the page (atomicity)")
	})

	// append on an existing page must behave exactly as before (content appended
	// at the end, no duplicate heading).
	t.Run("append_existing_unchanged", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Existing Page",
			"content": "Original body content.",
		})

		callTool(t, c, "patch_page", map[string]any{
			"page": "Existing Page",
			"operations": []map[string]any{
				{
					"op":      "append",
					"content": "\n\nAppended section.",
				},
			},
		})

		resp := getPageFull(t, c, "Existing Page")
		assert.Contains(t, resp.Content, "Original body content.")
		assert.Contains(t, resp.Content, "Appended section.")

		originalIdx := strings.Index(resp.Content, "Original body content.")
		appendedIdx := strings.Index(resp.Content, "Appended section.")
		assert.Greater(t, appendedIdx, originalIdx,
			"appended content must follow original content on an existing page")

		// Exactly one H1 heading must be present.
		headingCount := strings.Count(resp.Content, "# Existing Page")
		assert.Equal(t, 1, headingCount,
			"page must have exactly one H1 heading after appending to an existing page")
	})

	// prepend on an existing page must behave exactly as before (content inserted
	// after the heading, original body follows).
	t.Run("prepend_existing_unchanged", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Pre-existing Page",
			"content": "Original body content.",
		})

		callTool(t, c, "patch_page", map[string]any{
			"page": "Pre-existing Page",
			"operations": []map[string]any{
				{
					"op":      "prepend",
					"content": "Prepended note.\n\n",
				},
			},
		})

		resp := getPageFull(t, c, "Pre-existing Page")
		assert.True(t, strings.HasPrefix(resp.Content, "# Pre-existing Page"),
			"heading must remain at the top after prepend on an existing page")
		assert.Contains(t, resp.Content, "Prepended note.")
		assert.Contains(t, resp.Content, "Original body content.")

		prependedIdx := strings.Index(resp.Content, "Prepended note.")
		originalIdx := strings.Index(resp.Content, "Original body content.")
		assert.Greater(t, originalIdx, prependedIdx,
			"prepended content must appear before the original body")
	})

	// The response for a create-via-append must include the correct page name
	// and any wikilinks found in the appended content.
	t.Run("append_creates_page_returns_links_to", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		result := callTool(t, c, "patch_page", map[string]any{
			"page": "New Linked Page",
			"operations": []map[string]any{
				{
					"op":      "append",
					"content": "Connects to [[Alpha]] and [[Beta]] concepts.",
				},
			},
		})

		require.False(t, result.IsError)
		var resp patchPageResp
		parseJSON(t, result, &resp)
		assert.Equal(t, "New Linked Page", resp.Page)
		assert.ElementsMatch(t, []string{"Alpha", "Beta"}, resp.LinksTo,
			"links_to must include wikilinks from the appended content on a newly created page")
	})

	// The response for a create-via-prepend must include the correct page name
	// and any wikilinks found in the prepended content.
	t.Run("prepend_creates_page_returns_links_to", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		result := callTool(t, c, "patch_page", map[string]any{
			"page": "New Prepend Linked Page",
			"operations": []map[string]any{
				{
					"op":      "prepend",
					"content": "References [[Gamma]] and [[Delta]] topics.\n\n",
				},
			},
		})

		require.False(t, result.IsError)
		var resp patchPageResp
		parseJSON(t, result, &resp)
		assert.Equal(t, "New Prepend Linked Page", resp.Page)
		assert.ElementsMatch(t, []string{"Gamma", "Delta"}, resp.LinksTo,
			"links_to must include wikilinks from the prepended content on a newly created page")
	})

	// The auto-created page must appear in the search index immediately after
	// the call, just as write_page would index it.
	t.Run("append_creates_page_updates_index", func(t *testing.T) {
		t.Parallel()
		c, _, idx := setupTestServer(t)

		before := idx.Search("xyzzy987plugh", 10)
		assert.Empty(t, before, "distinctive term must not be indexed before the call")

		callTool(t, c, "patch_page", map[string]any{
			"page": "Index Creation Test",
			"operations": []map[string]any{
				{
					"op":      "append",
					"content": "The xyzzy987plugh technique is notable.",
				},
			},
		})

		after := idx.Search("xyzzy987plugh", 10)
		require.NotEmpty(t, after, "auto-created page must be indexed immediately after append create-on-write")
		assert.Equal(t, "Index Creation Test", after[0].Page)
	})

	// Same index check for a prepend create-on-write.
	t.Run("prepend_creates_page_updates_index", func(t *testing.T) {
		t.Parallel()
		c, _, idx := setupTestServer(t)

		before := idx.Search("quuxfrobnicator42", 10)
		assert.Empty(t, before, "distinctive term must not be indexed before the call")

		callTool(t, c, "patch_page", map[string]any{
			"page": "Prepend Index Test",
			"operations": []map[string]any{
				{
					"op":      "prepend",
					"content": "The quuxfrobnicator42 is key.\n\n",
				},
			},
		})

		after := idx.Search("quuxfrobnicator42", 10)
		require.NotEmpty(t, after, "auto-created page must be indexed immediately after prepend create-on-write")
		assert.Equal(t, "Prepend Index Test", after[0].Page)
	})

	// When auto-commit is enabled, creating a page via append must produce a
	// commit just as a normal patch_page call on an existing page does.
	t.Run("append_creates_page_auto_commits", func(t *testing.T) {
		t.Parallel()
		c, dir := setupTestServerAutoCommit(t)

		callTool(t, c, "patch_page", map[string]any{
			"page": "Auto Commit New Page",
			"operations": []map[string]any{
				{
					"op":      "append",
					"content": "Content for auto-commit test.",
				},
			},
		})

		assert.Equal(t, 1, commitCountAfterInit(t, dir),
			"auto-creating a page via append must produce exactly one commit")

		msg := latestCommitMessage(t, dir)
		assert.Contains(t, msg, "memento:", "commit message must carry the 'memento:' prefix")
		assert.Contains(t, msg, "Auto Commit New Page",
			"commit message must reference the newly created page name")
	})

	// When auto-commit is enabled, creating a page via prepend must also
	// produce a commit.
	t.Run("prepend_creates_page_auto_commits", func(t *testing.T) {
		t.Parallel()
		c, dir := setupTestServerAutoCommit(t)

		callTool(t, c, "patch_page", map[string]any{
			"page": "Auto Commit Prepend Page",
			"operations": []map[string]any{
				{
					"op":      "prepend",
					"content": "Content for prepend auto-commit test.\n\n",
				},
			},
		})

		assert.Equal(t, 1, commitCountAfterInit(t, dir),
			"auto-creating a page via prepend must produce exactly one commit")

		msg := latestCommitMessage(t, dir)
		assert.Contains(t, msg, "memento:", "commit message must carry the 'memento:' prefix")
		assert.Contains(t, msg, "Auto Commit Prepend Page",
			"commit message must reference the newly created page name")
	})

	// Wikilinks in the appended content of a newly created page must be
	// reflected in the link graph (i.e. appear in linked_from of the target
	// page after it too is created).
	t.Run("append_creates_page_wikilinks_indexed", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// First, create the link-target page so linked_from can be verified.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Link Target",
			"content": "This page is the target.",
		})

		// Now auto-create a new page that links to it.
		callTool(t, c, "patch_page", map[string]any{
			"page": "Link Source",
			"operations": []map[string]any{
				{
					"op":      "append",
					"content": "See [[Link Target]] for details.",
				},
			},
		})

		// The target page's linked_from must include the newly created source.
		resp := getPageFull(t, c, "Link Target")
		assert.Contains(t, resp.LinkedFrom, "Link Source",
			"link graph must reflect wikilinks in auto-created page's content")
	})
}
