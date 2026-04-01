package tools_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type patchPageResp struct {
	Page    string   `json:"page"`
	LinksTo []string `json:"links_to"`
}

// ---- TestPatchPage ---------------------------------------------------------

func TestPatchPage(t *testing.T) {
	t.Parallel()

	t.Run("replace_text", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "Enchanter is the only CC class in the game.",
		})

		result := callTool(t, c, "patch_page", map[string]any{
			"page": "Crowd Control",
			"operations": []map[string]any{
				{
					"op":  "replace",
					"old": "Enchanter is the only CC class in the game.",
					"new": "[[Enchanter]] is the primary CC class, though [[Bard]] has limited CC.",
				},
			},
		})

		// Verify the operation succeeded (not an error).
		require.False(t, result.IsError, "replace_text should succeed when old text is found exactly once")

		resp := getPageFull(t, c, "Crowd Control")
		assert.Contains(t, resp.Content, "[[Enchanter]] is the primary CC class")
		assert.NotContains(t, resp.Content, "Enchanter is the only CC class")
	})

	t.Run("replace_not_found", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Test Page",
			"content": "Some content here that exists.",
		})

		callToolExpectError(t, c, "patch_page", map[string]any{
			"page": "Test Page",
			"operations": []map[string]any{
				{
					"op":  "replace",
					"old": "THIS TEXT DOES NOT EXIST IN THE PAGE",
					"new": "replacement",
				},
			},
		})

		// Content must remain unchanged after the failed patch.
		resp := getPageFull(t, c, "Test Page")
		assert.Contains(t, resp.Content, "Some content here that exists.")
	})

	t.Run("replace_ambiguous", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Write a page where the target text appears more than once.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Ambiguous Page",
			"content": "duplicate text appears here.\nduplicate text appears again.",
		})

		// The replace must fail because the old text is ambiguous (appears twice).
		callToolExpectError(t, c, "patch_page", map[string]any{
			"page": "Ambiguous Page",
			"operations": []map[string]any{
				{
					"op":  "replace",
					"old": "duplicate text appears",
					"new": "replacement",
				},
			},
		})
	})

	t.Run("replace_lines", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Write a page with predictable line content.
		// After write_page the stored file is:
		//   line 1: "# Line Replace Test"
		//   line 2: "Content on line 1."
		//   line 3: "Content on line 2."
		//   line 4: "Content on line 3."
		//   ...
		callTool(t, c, "write_page", map[string]any{
			"page":    "Line Replace Test",
			"content": multilineContent(10),
		})

		callTool(t, c, "patch_page", map[string]any{
			"page": "Line Replace Test",
			"operations": []map[string]any{
				{
					"op":    "replace_lines",
					"lines": "3-5",
					"new":   "REPLACEMENT LINE A\nREPLACEMENT LINE B\nREPLACEMENT LINE C",
				},
			},
		})

		resp := getPageFull(t, c, "Line Replace Test")
		assert.Contains(t, resp.Content, "REPLACEMENT LINE A")
		assert.Contains(t, resp.Content, "REPLACEMENT LINE B")
		assert.Contains(t, resp.Content, "REPLACEMENT LINE C")
		// The original lines 3-5 (body lines 2-4) should be gone.
		assert.NotContains(t, resp.Content, "Content on line 2.")
		assert.NotContains(t, resp.Content, "Content on line 3.")
		assert.NotContains(t, resp.Content, "Content on line 4.")
		// Lines outside the replaced range must survive.
		assert.Contains(t, resp.Content, "Content on line 1.")
		assert.Contains(t, resp.Content, "Content on line 5.")
	})

	t.Run("replace_lines_single", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Single Line Replace",
			"content": multilineContent(10),
		})

		// Replace exactly one line (line 4 in the stored file = body line 3 = "Content on line 3.").
		callTool(t, c, "patch_page", map[string]any{
			"page": "Single Line Replace",
			"operations": []map[string]any{
				{
					"op":    "replace_lines",
					"lines": "4",
					"new":   "SINGLE REPLACEMENT",
				},
			},
		})

		resp := getPageFull(t, c, "Single Line Replace")
		assert.Contains(t, resp.Content, "SINGLE REPLACEMENT")
		assert.NotContains(t, resp.Content, "Content on line 3.")
		// Adjacent lines must be untouched.
		assert.Contains(t, resp.Content, "Content on line 2.")
		assert.Contains(t, resp.Content, "Content on line 4.")
	})

	t.Run("replace_lines_shorter", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Stored file lines after write_page:
		//   line 1: "# Shorter Replace"
		//   line 2: "First"
		//   line 3: "Second"
		//   line 4: "Third"
		//   line 5: "Fourth"
		//   line 6: "Fifth"
		callTool(t, c, "write_page", map[string]any{
			"page":    "Shorter Replace",
			"content": "First\nSecond\nThird\nFourth\nFifth",
		})

		// Replace 3 lines (2-4) with a single shorter line.
		callTool(t, c, "patch_page", map[string]any{
			"page": "Shorter Replace",
			"operations": []map[string]any{
				{
					"op":    "replace_lines",
					"lines": "2-4",
					"new":   "SHORTER",
				},
			},
		})

		resp := getPageFull(t, c, "Shorter Replace")
		assert.Contains(t, resp.Content, "SHORTER")
		assert.Contains(t, resp.Content, "Fourth")
		assert.Contains(t, resp.Content, "Fifth")
		assert.NotContains(t, resp.Content, "First")
		assert.NotContains(t, resp.Content, "Second")
		assert.NotContains(t, resp.Content, "Third")

		// Page should have fewer lines than before (was 6, now 4).
		lines := strings.Split(resp.Content, "\n")
		assert.Equal(t, 4, len(lines), "page should have 4 lines after replacing 3 lines with 1")
	})

	t.Run("replace_lines_longer", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Stored file lines:
		//   line 1: "# Longer Replace"
		//   line 2: "First"
		//   line 3: "Second"
		//   line 4: "Third"
		//   line 5: "Fourth"
		//   line 6: "Fifth"
		callTool(t, c, "write_page", map[string]any{
			"page":    "Longer Replace",
			"content": "First\nSecond\nThird\nFourth\nFifth",
		})

		// Replace 2 lines (2-3) with 4 lines.
		callTool(t, c, "patch_page", map[string]any{
			"page": "Longer Replace",
			"operations": []map[string]any{
				{
					"op":    "replace_lines",
					"lines": "2-3",
					"new":   "LONGER 1\nLONGER 2\nLONGER 3\nLONGER 4",
				},
			},
		})

		resp := getPageFull(t, c, "Longer Replace")
		assert.Contains(t, resp.Content, "LONGER 1")
		assert.Contains(t, resp.Content, "LONGER 2")
		assert.Contains(t, resp.Content, "LONGER 3")
		assert.Contains(t, resp.Content, "LONGER 4")
		// Lines after the replaced range must still be present.
		assert.Contains(t, resp.Content, "Third")
		assert.Contains(t, resp.Content, "Fourth")
		assert.Contains(t, resp.Content, "Fifth")
		assert.NotContains(t, resp.Content, "First")
		assert.NotContains(t, resp.Content, "Second")

		// Page should have more lines than before (was 6, now 8).
		lines := strings.Split(resp.Content, "\n")
		assert.Equal(t, 8, len(lines), "page should have 8 lines after replacing 2 lines with 4")
	})

	t.Run("append", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Append Test",
			"content": "Original body content.",
		})

		callTool(t, c, "patch_page", map[string]any{
			"page": "Append Test",
			"operations": []map[string]any{
				{
					"op":      "append",
					"content": "\n\n## Appendix\n\nAppended section content.",
				},
			},
		})

		resp := getPageFull(t, c, "Append Test")
		assert.Contains(t, resp.Content, "Appended section content.")

		// The appended content must appear AFTER the original body.
		originalIdx := strings.Index(resp.Content, "Original body content.")
		appendedIdx := strings.Index(resp.Content, "Appended section content.")
		require.NotEqual(t, -1, originalIdx, "original content should still be present")
		require.NotEqual(t, -1, appendedIdx, "appended content should be present")
		assert.Greater(t, appendedIdx, originalIdx,
			"appended content must appear after the original body")
	})

	t.Run("prepend", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Prepend Test",
			"content": "Original body content.",
		})

		callTool(t, c, "patch_page", map[string]any{
			"page": "Prepend Test",
			"operations": []map[string]any{
				{
					"op":      "prepend",
					"content": "Prepended note.\n\n",
				},
			},
		})

		resp := getPageFull(t, c, "Prepend Test")
		assert.Contains(t, resp.Content, "Prepended note.")

		// The heading must still be at the top.
		assert.True(t, strings.HasPrefix(resp.Content, "# Prepend Test"),
			"heading must remain at the top of the page after prepend")

		// The prepended content must appear BEFORE the original body.
		prependedIdx := strings.Index(resp.Content, "Prepended note.")
		originalIdx := strings.Index(resp.Content, "Original body content.")
		require.NotEqual(t, -1, prependedIdx, "prepended content should be present")
		require.NotEqual(t, -1, originalIdx, "original content should still be present")
		assert.Greater(t, originalIdx, prependedIdx,
			"prepended content must appear before the original body")
	})

	t.Run("multiple_ops", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Multi Op Test",
			"content": "First section content.\nSecond section content.",
		})

		result := callTool(t, c, "patch_page", map[string]any{
			"page": "Multi Op Test",
			"operations": []map[string]any{
				{
					"op":  "replace",
					"old": "First section content.",
					"new": "[[Modified First]] section.",
				},
				{
					"op":      "append",
					"content": "\n\nThird section added by append.",
				},
			},
		})

		require.False(t, result.IsError, "multiple_ops should succeed when all operations are valid")

		resp := getPageFull(t, c, "Multi Op Test")
		assert.Contains(t, resp.Content, "[[Modified First]] section.")
		assert.NotContains(t, resp.Content, "First section content.")
		assert.Contains(t, resp.Content, "Second section content.")
		assert.Contains(t, resp.Content, "Third section added by append.")
	})

	t.Run("atomicity", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Atomic Test",
			"content": "Unique original content here.",
		})

		// First op would succeed (old text exists once), but second op fails
		// (old text does not exist). The whole call must be rolled back.
		callToolExpectError(t, c, "patch_page", map[string]any{
			"page": "Atomic Test",
			"operations": []map[string]any{
				{
					"op":  "replace",
					"old": "Unique original content here.",
					"new": "Changed content — should be rolled back.",
				},
				{
					"op":  "replace",
					"old": "THIS TEXT DOES NOT EXIST",
					"new": "whatever",
				},
			},
		})

		// The first operation's change must NOT have been persisted.
		resp := getPageFull(t, c, "Atomic Test")
		assert.Contains(t, resp.Content, "Unique original content here.",
			"original content must be intact after a failed multi-op patch (atomicity)")
		assert.NotContains(t, resp.Content, "Changed content — should be rolled back.")
	})

	t.Run("line_numbers_pre_op", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// After write_page, the stored file is:
		//   line 1: "# Line Pre Op Test"
		//   line 2: "Block A line one"
		//   line 3: "Block A line two"
		//   line 4: "Middle separator"
		//   line 5: "Block B line one"
		//   line 6: "Block B line two"
		//
		// Both operations reference ORIGINAL line numbers. After op 1 replaces
		// lines 2-3 with a single line, the file shifts, but op 2 must still
		// correctly target original lines 5-6 (not whatever lands at positions 5-6
		// after op 1 is applied).
		callTool(t, c, "write_page", map[string]any{
			"page":    "Line Pre Op Test",
			"content": "Block A line one\nBlock A line two\nMiddle separator\nBlock B line one\nBlock B line two",
		})

		callTool(t, c, "patch_page", map[string]any{
			"page": "Line Pre Op Test",
			"operations": []map[string]any{
				{
					"op":    "replace_lines",
					"lines": "2-3",
					"new":   "New Block A",
				},
				{
					"op":    "replace_lines",
					"lines": "5-6",
					"new":   "New Block B first\nNew Block B second",
				},
			},
		})

		resp := getPageFull(t, c, "Line Pre Op Test")
		assert.Contains(t, resp.Content, "New Block A")
		assert.Contains(t, resp.Content, "Middle separator")
		assert.Contains(t, resp.Content, "New Block B first")
		assert.Contains(t, resp.Content, "New Block B second")

		// Original lines must be gone.
		assert.NotContains(t, resp.Content, "Block A line one")
		assert.NotContains(t, resp.Content, "Block A line two")
		assert.NotContains(t, resp.Content, "Block B line one")
		assert.NotContains(t, resp.Content, "Block B line two")

		// Order must be preserved: New Block A, then Middle separator, then New Block B.
		newAIdx := strings.Index(resp.Content, "New Block A")
		middleIdx := strings.Index(resp.Content, "Middle separator")
		newBIdx := strings.Index(resp.Content, "New Block B first")
		assert.Less(t, newAIdx, middleIdx, "New Block A must appear before Middle separator")
		assert.Less(t, middleIdx, newBIdx, "Middle separator must appear before New Block B")
	})

	t.Run("replace_page_not_found", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// replace is an existence-dependent op: the page must already exist.
		callToolExpectError(t, c, "patch_page", map[string]any{
			"page": "This Page Does Not Exist",
			"operations": []map[string]any{
				{
					"op":  "replace",
					"old": "some text",
					"new": "replacement",
				},
			},
		})
	})

	t.Run("returns_links_to", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "Basic crowd control page with no links.",
		})

		result := callTool(t, c, "patch_page", map[string]any{
			"page": "Crowd Control",
			"operations": []map[string]any{
				{
					"op":  "replace",
					"old": "Basic crowd control page with no links.",
					"new": "Use [[Enchanter]] and [[Bard]] for CC.",
				},
			},
		})

		var resp patchPageResp
		parseJSON(t, result, &resp)
		assert.Equal(t, "Crowd Control", resp.Page)
		assert.ElementsMatch(t, []string{"Enchanter", "Bard"}, resp.LinksTo,
			"patch_page response should reflect the updated wikilinks after the patch")
	})

	t.Run("updates_index", func(t *testing.T) {
		t.Parallel()
		c, _, idx := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "Basic crowd control page.",
		})

		// The distinctive term "xyzzy123splorf" is absent before the patch.
		before := idx.Search("xyzzy123splorf", 10)
		assert.Empty(t, before, "term should not be indexed before patch")

		callTool(t, c, "patch_page", map[string]any{
			"page": "Crowd Control",
			"operations": []map[string]any{
				{
					"op":      "append",
					"content": "\n\nThe xyzzy123splorf technique is notable here.",
				},
			},
		})

		after := idx.Search("xyzzy123splorf", 10)
		require.NotEmpty(t, after, "patch_page must update the search index immediately")
		assert.Equal(t, "Crowd Control", after[0].Page)
	})
}
