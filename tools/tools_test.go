package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/edgetools/memento/tools"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Response types --------------------------------------------------------
// These mirror the JSON schemas from MVP_DESIGN.md exactly.

type writePageResp struct {
	Page    string   `json:"page"`
	LinksTo []string `json:"links_to"`
}

type getPageSection struct {
	Lines   string `json:"lines"`
	Content string `json:"content"`
}

type getPageFullResp struct {
	Page       string   `json:"page"`
	Content    string   `json:"content"`
	TotalLines int      `json:"total_lines"`
	LinksTo    []string `json:"links_to"`
	LinkedFrom []string `json:"linked_from"`
}

type getPageRangeResp struct {
	Page       string           `json:"page"`
	Sections   []getPageSection `json:"sections"`
	TotalLines int              `json:"total_lines"`
	LinksTo    []string         `json:"links_to"`
	LinkedFrom []string         `json:"linked_from"`
}

type deletePageResp struct {
	Page string `json:"page"`
}

// ---- Test helpers ----------------------------------------------------------

// setupTestServer creates an isolated environment: a temp-dir-backed store, a fresh
// index, and a started+initialized in-process MCP client with all tools registered.
// The client is closed automatically on test cleanup.
// Returning the store and index lets individual tests verify state directly
// without depending on the search MCP tool (added in D6).
func setupTestServer(t *testing.T) (*client.Client, *pages.Store, *index.Index) {
	t.Helper()

	dir := t.TempDir()
	store := pages.NewStore(dir)
	idx := index.NewIndex()

	s := server.NewMCPServer("memento-test", "0.0.0", server.WithToolCapabilities(true))
	tools.Register(s, store, idx)

	c, err := client.NewInProcessClient(s)
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })

	require.NoError(t, c.Start(context.Background()))

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "memento-test-client", Version: "0.0.0"}
	_, err = c.Initialize(context.Background(), initReq)
	require.NoError(t, err)

	return c, store, idx
}

// callTool invokes a named MCP tool and fails the test on any transport-level error.
func callTool(t *testing.T, c *client.Client, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	result, err := c.CallTool(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	return result
}

// callToolExpectError calls a tool and asserts the result carries IsError=true.
func callToolExpectError(t *testing.T, c *client.Client, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	result := callTool(t, c, name, args)
	require.True(t, result.IsError,
		"expected IsError=true in tool result, but got a success response")
	return result
}

// parseJSON extracts and decodes the first text content item from a tool result.
func parseJSON(t *testing.T, result *mcp.CallToolResult, v any) {
	t.Helper()
	require.NotEmpty(t, result.Content, "tool result has no content items")
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok, "first content item is not text content")
	err := json.Unmarshal([]byte(textContent.Text), v)
	require.NoError(t, err, "failed to unmarshal tool result JSON: %s", textContent.Text)
}

// multilineContent builds a body string with n numbered lines, used to create
// pages with predictable line ranges for get_page range tests.
func multilineContent(n int) string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("Content on line %d.", i+1)
	}
	return strings.Join(lines, "\n")
}

// ---- TestWritePage ---------------------------------------------------------

func TestWritePage(t *testing.T) {
	t.Parallel()

	t.Run("creates_new", func(t *testing.T) {
		t.Parallel()
		c, store, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "The enchanter is a utility class.",
		})

		assert.True(t, store.Exists("Enchanter"), "page should exist in the store after write")
	})

	t.Run("returns_links_to", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		result := callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "Specializes in [[Mez]] and [[Haste]] spells.",
		})

		var resp writePageResp
		parseJSON(t, result, &resp)
		assert.Equal(t, "Enchanter", resp.Page)
		assert.ElementsMatch(t, []string{"Mez", "Haste"}, resp.LinksTo)
	})

	t.Run("replaces_existing", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Bard",
			"content": "Original bard content.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Bard",
			"content": "Replaced bard content.",
		})

		result := callTool(t, c, "get_page", map[string]any{"page": "Bard"})
		var resp getPageFullResp
		parseJSON(t, result, &resp)
		assert.Contains(t, resp.Content, "Replaced bard content.")
		assert.NotContains(t, resp.Content, "Original bard content.")
	})

	t.Run("case_insensitive_overwrite", func(t *testing.T) {
		t.Parallel()
		c, store, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "First write.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "crowd control",
			"content": "Second write.",
		})

		// Both writes target the same logical page — only one file should exist.
		count := 0
		for _, p := range store.Scan() {
			if pages.NamesMatch(p.Name, "Crowd Control") {
				count++
			}
		}
		assert.Equal(t, 1, count, "case-insensitive overwrite should produce exactly one page")
	})

	t.Run("heading_managed", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Content has no heading — MCP must inject the correct h1.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Mez",
			"content": "Mesmerize immobilizes enemies temporarily.",
		})

		result := callTool(t, c, "get_page", map[string]any{"page": "Mez"})
		var resp getPageFullResp
		parseJSON(t, result, &resp)
		assert.True(t, strings.HasPrefix(resp.Content, "# Mez"),
			"content must start with the correct h1 heading")
	})

	t.Run("agent_heading_replaced", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Agent-supplied heading is wrong — MCP must replace it with the page name.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "# Wrong Title\n\nThe correct body text.",
		})

		result := callTool(t, c, "get_page", map[string]any{"page": "Enchanter"})
		var resp getPageFullResp
		parseJSON(t, result, &resp)
		assert.Contains(t, resp.Content, "# Enchanter", "page-name heading must replace the agent-supplied one")
		assert.NotContains(t, resp.Content, "# Wrong Title")
	})

	t.Run("empty_content", func(t *testing.T) {
		t.Parallel()
		c, store, _ := setupTestServer(t)

		result := callTool(t, c, "write_page", map[string]any{
			"page":    "Empty Page",
			"content": "",
		})

		var resp writePageResp
		parseJSON(t, result, &resp)
		assert.Equal(t, "Empty Page", resp.Page)
		assert.True(t, store.Exists("Empty Page"), "page with empty body should still be created")
	})

	t.Run("updates_search_index", func(t *testing.T) {
		t.Parallel()
		c, _, idx := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Necromancer",
			"content": "Necromancers summon undead minions to fight for them.",
		})

		// Query the index directly — no dependency on the search MCP tool from D6.
		results := idx.Search("necromancer", 10)
		require.NotEmpty(t, results, "page must be searchable immediately after write_page")
		assert.Equal(t, "Necromancer", results[0].Page)
	})

	t.Run("missing_page_errors", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Omitting the required 'page' argument must produce a tool error.
		callToolExpectError(t, c, "write_page", map[string]any{
			"content": "Content with no page name.",
		})
	})

	t.Run("missing_content_errors", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		// Omitting the required 'content' argument must produce a tool error.
		callToolExpectError(t, c, "write_page", map[string]any{
			"page": "Page Without Content",
		})
	})
}

// ---- TestGetPage -----------------------------------------------------------

func TestGetPage(t *testing.T) {
	t.Parallel()

	t.Run("full_content", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Pulling",
			"content": "Pull strategy depends on [[Crowd Control]].\n\nSee also [[Enchanter]].",
		})
		// Write a page that links to Pulling so linked_from is non-empty.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Dungeon Strategy",
			"content": "See [[Pulling]] for the pull approach.",
		})

		result := callTool(t, c, "get_page", map[string]any{"page": "Pulling"})
		var resp getPageFullResp
		parseJSON(t, result, &resp)

		assert.Equal(t, "Pulling", resp.Page)
		assert.NotEmpty(t, resp.Content)
		assert.Greater(t, resp.TotalLines, 0)
		assert.ElementsMatch(t, []string{"Crowd Control", "Enchanter"}, resp.LinksTo)
		assert.Contains(t, resp.LinkedFrom, "Dungeon Strategy")
	})

	t.Run("case_insensitive", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Root",
			"content": "Root spells immobilize enemies in place.",
		})

		// Look up using different casing.
		result := callTool(t, c, "get_page", map[string]any{"page": "root"})
		var resp getPageFullResp
		parseJSON(t, result, &resp)
		assert.Equal(t, "Root", resp.Page)
		assert.NotEmpty(t, resp.Content)
	})

	t.Run("not_found_errors", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callToolExpectError(t, c, "get_page", map[string]any{
			"page": "This Page Does Not Exist",
		})
	})

	t.Run("line_range_single", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Range Test",
			"content": multilineContent(20),
		})

		result := callTool(t, c, "get_page", map[string]any{
			"page":  "Range Test",
			"lines": []string{"3-5"},
		})

		var resp getPageRangeResp
		parseJSON(t, result, &resp)
		require.Len(t, resp.Sections, 1, "one range should produce one section")
		assert.Equal(t, "3-5", resp.Sections[0].Lines)
		assert.NotEmpty(t, resp.Sections[0].Content)
		assert.Greater(t, resp.TotalLines, 0)
	})

	t.Run("line_range_multiple", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Multi Range Test",
			"content": multilineContent(25),
		})

		result := callTool(t, c, "get_page", map[string]any{
			"page":  "Multi Range Test",
			"lines": []string{"1-3", "8-10"},
		})

		var resp getPageRangeResp
		parseJSON(t, result, &resp)
		require.Len(t, resp.Sections, 2, "two ranges should produce two sections")
		assert.Equal(t, "1-3", resp.Sections[0].Lines)
		assert.Equal(t, "8-10", resp.Sections[1].Lines)
		assert.NotEmpty(t, resp.Sections[0].Content)
		assert.NotEmpty(t, resp.Sections[1].Content)
	})

	t.Run("line_range_single_line", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Single Line Test",
			"content": multilineContent(10),
		})

		result := callTool(t, c, "get_page", map[string]any{
			"page":  "Single Line Test",
			"lines": []string{"4"},
		})

		var resp getPageRangeResp
		parseJSON(t, result, &resp)
		require.Len(t, resp.Sections, 1)
		assert.Equal(t, "4", resp.Sections[0].Lines)
		assert.NotEmpty(t, resp.Sections[0].Content)
	})

	t.Run("line_range_out_of_bounds", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Short Page",
			"content": "Line one.\nLine two.",
		})

		// Requesting lines far beyond the page's length must return a tool error.
		callToolExpectError(t, c, "get_page", map[string]any{
			"page":  "Short Page",
			"lines": []string{"100-200"},
		})
	})

	t.Run("links_to", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Party Composition",
			"content": "A party needs [[Tank]], [[Healer]], and [[Enchanter]] roles.",
		})

		result := callTool(t, c, "get_page", map[string]any{"page": "Party Composition"})
		var resp getPageFullResp
		parseJSON(t, result, &resp)
		assert.ElementsMatch(t, []string{"Tank", "Healer", "Enchanter"}, resp.LinksTo)
	})

	t.Run("linked_from", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Mez",
			"content": "Mesmerize is a crowd control type.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "Includes [[Mez]] and root effects.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter Guide",
			"content": "Primary tool is the [[Mez]] ability.",
		})

		result := callTool(t, c, "get_page", map[string]any{"page": "Mez"})
		var resp getPageFullResp
		parseJSON(t, result, &resp)
		assert.ElementsMatch(t, []string{"Crowd Control", "Enchanter Guide"}, resp.LinkedFrom)
	})

	t.Run("total_lines_correct", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Line Counter",
			"content": "Line one.\nLine two.\nLine three.",
		})

		result := callTool(t, c, "get_page", map[string]any{"page": "Line Counter"})
		var resp getPageFullResp
		parseJSON(t, result, &resp)

		// total_lines must match the actual newline-delimited line count of the content.
		actualLines := strings.Count(resp.Content, "\n") + 1
		assert.Equal(t, actualLines, resp.TotalLines,
			"total_lines must equal the number of lines in the returned content")
	})
}

// ---- TestDeletePage --------------------------------------------------------

func TestDeletePage(t *testing.T) {
	t.Parallel()

	t.Run("removes", func(t *testing.T) {
		t.Parallel()
		c, store, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Obsolete Concept",
			"content": "This page will be removed.",
		})

		callTool(t, c, "delete_page", map[string]any{"page": "Obsolete Concept"})

		assert.False(t, store.Exists("Obsolete Concept"), "page must not exist in store after delete")
		callToolExpectError(t, c, "get_page", map[string]any{"page": "Obsolete Concept"})
	})

	t.Run("case_insensitive", func(t *testing.T) {
		t.Parallel()
		c, store, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Foo Page",
			"content": "Some content.",
		})

		// Delete using different casing than the page was written with.
		callTool(t, c, "delete_page", map[string]any{"page": "foo page"})

		assert.False(t, store.Exists("Foo Page"), "case-insensitive delete should remove the page")
	})

	t.Run("not_found_errors", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callToolExpectError(t, c, "delete_page", map[string]any{
			"page": "Nonexistent Page",
		})
	})

	t.Run("removes_from_index", func(t *testing.T) {
		t.Parallel()
		c, _, idx := setupTestServer(t)

		// Use a distinctive term unlikely to appear in other test pages.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Obsolete Concept",
			"content": "This page uses the term flibbertigibbet extensively.",
		})

		// Confirm it is indexed before deletion.
		before := idx.Search("flibbertigibbet", 10)
		require.NotEmpty(t, before, "page must appear in the index before deletion")

		callTool(t, c, "delete_page", map[string]any{"page": "Obsolete Concept"})

		// After deletion the page must not appear in search results.
		after := idx.Search("flibbertigibbet", 10)
		for _, r := range after {
			assert.NotEqual(t, "Obsolete Concept", r.Page,
				"deleted page must not appear in index search results")
		}
	})

	t.Run("preserves_broken_links", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Necromancer",
			"content": "Primary ability is [[Death Coil]].",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Death Coil",
			"content": "A damage and heal spell.",
		})

		callTool(t, c, "delete_page", map[string]any{"page": "Death Coil"})

		// Necromancer must still exist and still contain the now-broken wikilink.
		result := callTool(t, c, "get_page", map[string]any{"page": "Necromancer"})
		var resp getPageFullResp
		parseJSON(t, result, &resp)
		assert.Contains(t, resp.Content, "[[Death Coil]]",
			"delete_page must not modify other pages that link to the deleted page")
	})

	t.Run("returns_name", func(t *testing.T) {
		t.Parallel()
		c, _, _ := setupTestServer(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Temporary Page",
			"content": "Short-lived content.",
		})

		result := callTool(t, c, "delete_page", map[string]any{"page": "Temporary Page"})
		var resp deletePageResp
		parseJSON(t, result, &resp)
		assert.Equal(t, "Temporary Page", resp.Page)
	})
}
