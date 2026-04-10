package tools_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/edgetools/memento/tools"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Response types for timestamp feature ------------------------------------

// getPageFullRespWithTS mirrors the get_page full-page JSON schema with last_updated.
type getPageFullRespWithTS struct {
	Page        string   `json:"page"`
	Content     string   `json:"content"`
	TotalLines  int      `json:"total_lines"`
	LastUpdated string   `json:"last_updated"`
	LinksTo     []string `json:"links_to"`
	LinkedFrom  []string `json:"linked_from"`
}

// getPageRangeRespWithTS mirrors the get_page line-range JSON schema with last_updated.
type getPageRangeRespWithTS struct {
	Page        string           `json:"page"`
	Sections    []getPageSection `json:"sections"`
	TotalLines  int              `json:"total_lines"`
	LastUpdated string           `json:"last_updated"`
	LinksTo     []string         `json:"links_to"`
	LinkedFrom  []string         `json:"linked_from"`
}

// searchResultWithTS mirrors searchResult but includes last_updated.
type searchResultWithTS struct {
	Page        string   `json:"page"`
	Relevance   float64  `json:"relevance"`
	LastUpdated string   `json:"last_updated"`
	Snippet     string   `json:"snippet"`
	Line        int      `json:"line"`
	LinkedPages []string `json:"linked_pages"`
}

// searchLinkedPageDetailWithTS mirrors searchLinkedPageDetail but includes last_updated.
type searchLinkedPageDetailWithTS struct {
	Page        string `json:"page"`
	LastUpdated string `json:"last_updated"`
	Snippet     string `json:"snippet"`
	Line        int    `json:"line"`
}

// searchRespWithTS mirrors searchResp but uses types that include last_updated.
type searchRespWithTS struct {
	Results           []searchResultWithTS           `json:"results"`
	LinkedPageDetails []searchLinkedPageDetailWithTS `json:"linked_page_details"`
}

// listPageEntry is the object format returned by list_pages for newest/oldest sort.
type listPageEntry struct {
	Page        string `json:"page"`
	LastUpdated string `json:"last_updated"`
}

// listPagesTimestampResp is the list_pages response for newest/oldest sort modes,
// where pages is an array of objects rather than flat strings.
type listPagesTimestampResp struct {
	Pages  []listPageEntry `json:"pages"`
	Total  int             `json:"total"`
	Offset int             `json:"offset"`
	Limit  int             `json:"limit"`
}

// ---- Helpers for timestamp tests ---------------------------------------------

// setupTestServerAndDir is like setupTestServer but also returns the content
// directory path, needed for timestamp manipulation via os.Chtimes.
func setupTestServerAndDir(t *testing.T) (*client.Client, *pages.Store, *index.Index, string) {
	t.Helper()

	dir := t.TempDir()
	store := pages.NewStore(dir)
	idx := index.NewIndex()

	s := server.NewMCPServer("memento-test-ts", "0.0.0", server.WithToolCapabilities(true))
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

	return c, store, idx, dir
}

// setupTestServerInGitDir creates a git-initialised content directory but uses
// the standard (non-auto-commit) server registration. This simulates a git repo
// where pages are written but not automatically committed — so files are untracked
// and the mtime fallback applies.
func setupTestServerInGitDir(t *testing.T) (*client.Client, string) {
	t.Helper()

	dir := t.TempDir()
	initGitRepo(t, dir)

	store := pages.NewStore(dir)
	idx := index.NewIndex()

	s := server.NewMCPServer("memento-test-gitdir-nac", "0.0.0", server.WithToolCapabilities(true))
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

	return c, dir
}

// setFileMtime sets the access and modification time of path to mtime.
// Fails the test immediately on any OS error.
func setFileMtime(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	require.NoError(t, os.Chtimes(path, mtime, mtime),
		"os.Chtimes failed for %s", path)
}

// pageFilePath returns the expected filesystem path for a page file in dir.
// Pages are stored as {pageName}.md in a flat content directory.
func pageFilePath(dir, pageName string) string {
	return filepath.Join(dir, pageName+".md")
}

// assertISO8601UTC asserts that ts is a non-empty, valid RFC 3339 / ISO 8601
// timestamp in UTC (must end in "Z", not an offset like "+00:00").
func assertISO8601UTC(t *testing.T, ts string) {
	t.Helper()
	require.NotEmpty(t, ts, "last_updated must not be empty")
	_, err := time.Parse(time.RFC3339, ts)
	require.NoError(t, err, "last_updated %q must be valid RFC 3339 / ISO 8601", ts)
	assert.True(t, strings.HasSuffix(ts, "Z"),
		"last_updated %q must end in Z to indicate UTC", ts)
}

// ---- TestGetPageLastUpdated -------------------------------------------------

func TestGetPageLastUpdated(t *testing.T) {
	t.Parallel()

	// present_in_full_response verifies that get_page (no lines param) includes
	// last_updated in the JSON response after a page is written.
	t.Run("present_in_full_response", func(t *testing.T) {
		t.Parallel()
		c, _, _, _ := setupTestServerAndDir(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Full Response Ts Test",
			"content": "Content for full response timestamp test.",
		})

		result := callTool(t, c, "get_page", map[string]any{"page": "Full Response Ts Test"})
		var resp getPageFullRespWithTS
		parseJSON(t, result, &resp)

		assert.NotEmpty(t, resp.LastUpdated,
			"get_page full response must include last_updated after a page is written")
	})

	// present_in_range_response verifies that get_page with a lines parameter also
	// includes last_updated in the JSON response.
	t.Run("present_in_range_response", func(t *testing.T) {
		t.Parallel()
		c, _, _, _ := setupTestServerAndDir(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Range Response Ts Test",
			"content": multilineContent(10),
		})

		result := callTool(t, c, "get_page", map[string]any{
			"page":  "Range Response Ts Test",
			"lines": []string{"2-4"},
		})
		var resp getPageRangeRespWithTS
		parseJSON(t, result, &resp)

		assert.NotEmpty(t, resp.LastUpdated,
			"get_page range response must include last_updated")
	})

	// is_iso8601_utc_format verifies that last_updated is a valid RFC 3339 / ISO 8601
	// string in UTC, ending in "Z" not an offset like "+00:00".
	t.Run("is_iso8601_utc_format", func(t *testing.T) {
		t.Parallel()
		c, _, _, _ := setupTestServerAndDir(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Format Ts Test",
			"content": "Content for timestamp format test.",
		})

		result := callTool(t, c, "get_page", map[string]any{"page": "Format Ts Test"})
		var resp getPageFullRespWithTS
		parseJSON(t, result, &resp)

		assertISO8601UTC(t, resp.LastUpdated)
	})

	// git_commit_time_preferred_over_mtime verifies that when the content directory
	// is inside a git repo and the file has been committed, the git commit timestamp
	// is used as last_updated — not the filesystem mtime. The test verifies this by
	// setting the mtime to a far-future value after the commit and confirming
	// last_updated still reflects the commit time, not the future mtime.
	t.Run("git_commit_time_preferred_over_mtime", func(t *testing.T) {
		t.Parallel()
		c, dir := setupTestServerAutoCommit(t)

		before := time.Now().UTC().Add(-time.Second)
		callTool(t, c, "write_page", map[string]any{
			"page":    "Git Pref Ts Test",
			"content": "Content for git preference test.",
		})
		after := time.Now().UTC().Add(time.Second)

		// Set the file's mtime to far in the future. If the implementation
		// erroneously uses mtime, last_updated will reflect this future value.
		futureTime := time.Now().UTC().Add(24 * time.Hour)
		setFileMtime(t, pageFilePath(dir, "Git Pref Ts Test"), futureTime)

		result := callTool(t, c, "get_page", map[string]any{"page": "Git Pref Ts Test"})
		var resp getPageFullRespWithTS
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.LastUpdated)
		ts, err := time.Parse(time.RFC3339, resp.LastUpdated)
		require.NoError(t, err)

		// last_updated must be the git commit time (bracketed by before/after),
		// not the far-future mtime we set after the commit.
		assert.True(t, !ts.Before(before),
			"last_updated %q must be at or after the write started", resp.LastUpdated)
		assert.True(t, ts.Before(after.Add(time.Second)),
			"last_updated %q must reflect the git commit time, not the far-future mtime", resp.LastUpdated)
	})

	// mtime_used_when_not_in_git_repo verifies that when the content directory is
	// not inside any git repo, the filesystem mtime is used as last_updated.
	t.Run("mtime_used_when_not_in_git_repo", func(t *testing.T) {
		t.Parallel()
		c, _, _, dir := setupTestServerAndDir(t) // plain temp dir — no git

		callTool(t, c, "write_page", map[string]any{
			"page":    "Mtime Fallback Test",
			"content": "Content for mtime fallback test.",
		})

		// Pin the mtime to a specific past time.
		knownTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
		setFileMtime(t, pageFilePath(dir, "Mtime Fallback Test"), knownTime)

		result := callTool(t, c, "get_page", map[string]any{"page": "Mtime Fallback Test"})
		var resp getPageFullRespWithTS
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.LastUpdated)
		ts, err := time.Parse(time.RFC3339, resp.LastUpdated)
		require.NoError(t, err)

		assert.Equal(t, knownTime, ts.UTC().Truncate(time.Second),
			"last_updated must equal the file's mtime when the directory is not a git repo")
	})

	// mtime_used_for_uncommitted_file_in_git_repo verifies that when the content
	// directory IS inside a git repo but the specific file has never been committed
	// (e.g. a newly created page not yet staged), the filesystem mtime is used as
	// the fallback source rather than returning no timestamp.
	t.Run("mtime_used_for_uncommitted_file_in_git_repo", func(t *testing.T) {
		t.Parallel()
		c, dir := setupTestServerInGitDir(t) // git repo, no auto-commit

		callTool(t, c, "write_page", map[string]any{
			"page":    "Uncommitted Page",
			"content": "This file lives in a git repo but has never been committed.",
		})

		// Pin the mtime so we can assert on an exact value.
		knownTime := time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC)
		setFileMtime(t, pageFilePath(dir, "Uncommitted Page"), knownTime)

		result := callTool(t, c, "get_page", map[string]any{"page": "Uncommitted Page"})
		var resp getPageFullRespWithTS
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.LastUpdated)
		ts, err := time.Parse(time.RFC3339, resp.LastUpdated)
		require.NoError(t, err)

		assert.Equal(t, knownTime, ts.UTC().Truncate(time.Second),
			"last_updated must use mtime when the file is in a git repo but not yet committed")
	})

	// rename_updates_last_updated verifies that rename_page advances last_updated.
	// Renaming rewrites the file's heading (# New Name), which is a content write
	// and must update the timestamp.
	t.Run("rename_updates_last_updated", func(t *testing.T) {
		t.Parallel()
		c, _, _, dir := setupTestServerAndDir(t) // non-git for deterministic mtime control

		callTool(t, c, "write_page", map[string]any{
			"page":    "Rename Ts Source",
			"content": "Content for rename timestamp test.",
		})

		// Pin the mtime to a specific past time so we can tell whether the rename
		// advanced it.
		pastTime := time.Date(2022, 3, 10, 8, 0, 0, 0, time.UTC)
		setFileMtime(t, pageFilePath(dir, "Rename Ts Source"), pastTime)

		callTool(t, c, "rename_page", map[string]any{
			"page":     "Rename Ts Source",
			"new_name": "Rename Ts Target",
		})

		// The renamed page rewrites the heading, so last_updated must be newer
		// than the pinned past time.
		result := callTool(t, c, "get_page", map[string]any{"page": "Rename Ts Target"})
		var resp getPageFullRespWithTS
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.LastUpdated)
		ts, err := time.Parse(time.RFC3339, resp.LastUpdated)
		require.NoError(t, err)

		assert.True(t, ts.After(pastTime),
			"rename_page rewrites the heading and must advance last_updated past the pinned time")
	})
}

// ---- TestSearchLastUpdated --------------------------------------------------

func TestSearchLastUpdated(t *testing.T) {
	t.Parallel()

	// present_in_search_results verifies that each entry in the search results
	// array includes a valid last_updated timestamp.
	t.Run("present_in_search_results", func(t *testing.T) {
		t.Parallel()
		c, _, _, _ := setupTestServerAndDir(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Search Ts Page",
			"content": "This page contains the unique term zyphronite for timestamp search testing.",
		})

		result := callTool(t, c, "search", map[string]any{"query": "zyphronite"})
		var resp searchRespWithTS
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.Results, "search must return at least one result")

		found := false
		for _, r := range resp.Results {
			if r.Page == "Search Ts Page" {
				assertISO8601UTC(t, r.LastUpdated)
				found = true
				break
			}
		}
		assert.True(t, found, "Search Ts Page must appear in search results with a valid last_updated")
	})

	// present_in_linked_page_details verifies that entries in linked_page_details
	// also carry a valid last_updated timestamp.
	t.Run("present_in_linked_page_details", func(t *testing.T) {
		t.Parallel()
		c, _, _, _ := setupTestServerAndDir(t)

		// "Linked Detail Page" is the target; "Linker Page" references it and
		// introduces the search term vraxilon. The search should surface
		// "Linked Detail Page" in linked_page_details (or results), with last_updated.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Linked Detail Page",
			"content": "A page about theoretical mechanics.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Linker Page",
			"content": "This page discusses vraxilon and links to [[Linked Detail Page]].",
		})

		result := callTool(t, c, "search", map[string]any{"query": "vraxilon"})
		var resp searchRespWithTS
		parseJSON(t, result, &resp)

		// "Linked Detail Page" must appear somewhere with last_updated set.
		foundInResults := false
		for _, r := range resp.Results {
			if r.Page == "Linked Detail Page" {
				assertISO8601UTC(t, r.LastUpdated)
				foundInResults = true
				break
			}
		}

		foundInDetails := false
		for _, d := range resp.LinkedPageDetails {
			if d.Page == "Linked Detail Page" {
				assertISO8601UTC(t, d.LastUpdated)
				foundInDetails = true
				break
			}
		}

		assert.True(t, foundInResults || foundInDetails,
			"Linked Detail Page must appear in results or linked_page_details with a valid last_updated")
	})
}

// ---- TestListPagesTimestamp -------------------------------------------------

func TestListPagesTimestamp(t *testing.T) {
	t.Parallel()

	// ---- output format: newest and oldest return objects ----------------------

	// sort_newest_returns_objects verifies that sort_by newest changes the pages
	// field from a flat array of strings to an array of {page, last_updated} objects.
	t.Run("sort_newest_returns_objects", func(t *testing.T) {
		t.Parallel()
		c, _, _, _ := setupTestServerAndDir(t)

		callTool(t, c, "write_page", map[string]any{"page": "Ts Object Newest", "content": "Content."})

		result := callTool(t, c, "list_pages", map[string]any{"sort_by": "newest"})

		// Parsing into listPagesTimestampResp (Pages []listPageEntry) will fail if
		// the server returns flat strings for newest.
		var resp listPagesTimestampResp
		parseJSON(t, result, &resp)

		require.Len(t, resp.Pages, 1)
		assert.Equal(t, "Ts Object Newest", resp.Pages[0].Page,
			"page entry must carry the page name")
		assert.NotEmpty(t, resp.Pages[0].LastUpdated,
			"page entry must carry last_updated")
	})

	// sort_oldest_returns_objects verifies that sort_by oldest also returns the
	// object format rather than flat strings.
	t.Run("sort_oldest_returns_objects", func(t *testing.T) {
		t.Parallel()
		c, _, _, _ := setupTestServerAndDir(t)

		callTool(t, c, "write_page", map[string]any{"page": "Ts Object Oldest", "content": "Content."})

		result := callTool(t, c, "list_pages", map[string]any{"sort_by": "oldest"})
		var resp listPagesTimestampResp
		parseJSON(t, result, &resp)

		require.Len(t, resp.Pages, 1)
		assert.Equal(t, "Ts Object Oldest", resp.Pages[0].Page)
		assert.NotEmpty(t, resp.Pages[0].LastUpdated)
	})

	// objects_have_valid_iso8601_last_updated verifies that the last_updated field
	// on each page entry in newest/oldest output is a valid ISO 8601 UTC timestamp.
	t.Run("objects_have_valid_iso8601_last_updated", func(t *testing.T) {
		t.Parallel()
		c, _, _, _ := setupTestServerAndDir(t)

		callTool(t, c, "write_page", map[string]any{"page": "Ts Format List Check", "content": "Content."})

		result := callTool(t, c, "list_pages", map[string]any{"sort_by": "newest"})
		var resp listPagesTimestampResp
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.Pages)
		assertISO8601UTC(t, resp.Pages[0].LastUpdated)
	})

	// ---- sort order -----------------------------------------------------------

	// sort_newest_descending_order verifies that sort_by newest returns pages in
	// descending order by last_updated (most recently updated first).
	t.Run("sort_newest_descending_order", func(t *testing.T) {
		t.Parallel()
		c, _, _, dir := setupTestServerAndDir(t)

		callTool(t, c, "write_page", map[string]any{"page": "Oldest Newest Test", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Middle Newest Test", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Newest Newest Test", "content": "Content."})

		// Assign distinct mtimes: Oldest < Middle < Newest.
		oldTime := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
		midTime := time.Date(2022, 6, 15, 12, 0, 0, 0, time.UTC)
		newTime := time.Date(2024, 3, 20, 8, 30, 0, 0, time.UTC)

		setFileMtime(t, pageFilePath(dir, "Oldest Newest Test"), oldTime)
		setFileMtime(t, pageFilePath(dir, "Middle Newest Test"), midTime)
		setFileMtime(t, pageFilePath(dir, "Newest Newest Test"), newTime)

		result := callTool(t, c, "list_pages", map[string]any{"sort_by": "newest"})
		var resp listPagesTimestampResp
		parseJSON(t, result, &resp)

		require.Len(t, resp.Pages, 3)
		assert.Equal(t, "Newest Newest Test", resp.Pages[0].Page,
			"most recently updated page must be first with sort_by newest")
		assert.Equal(t, "Middle Newest Test", resp.Pages[1].Page)
		assert.Equal(t, "Oldest Newest Test", resp.Pages[2].Page,
			"oldest page must be last with sort_by newest")
	})

	// sort_oldest_ascending_order verifies that sort_by oldest returns pages in
	// ascending order by last_updated (least recently updated first).
	t.Run("sort_oldest_ascending_order", func(t *testing.T) {
		t.Parallel()
		c, _, _, dir := setupTestServerAndDir(t)

		callTool(t, c, "write_page", map[string]any{"page": "Old Oldest Test", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Mid Oldest Test", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "New Oldest Test", "content": "Content."})

		oldTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		midTime := time.Date(2021, 6, 1, 0, 0, 0, 0, time.UTC)
		newTime := time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC)

		setFileMtime(t, pageFilePath(dir, "Old Oldest Test"), oldTime)
		setFileMtime(t, pageFilePath(dir, "Mid Oldest Test"), midTime)
		setFileMtime(t, pageFilePath(dir, "New Oldest Test"), newTime)

		result := callTool(t, c, "list_pages", map[string]any{"sort_by": "oldest"})
		var resp listPagesTimestampResp
		parseJSON(t, result, &resp)

		require.Len(t, resp.Pages, 3)
		assert.Equal(t, "Old Oldest Test", resp.Pages[0].Page,
			"oldest page must be first with sort_by oldest")
		assert.Equal(t, "Mid Oldest Test", resp.Pages[1].Page)
		assert.Equal(t, "New Oldest Test", resp.Pages[2].Page,
			"most recently updated page must be last with sort_by oldest")
	})

	// ---- consistency between list_pages and get_page -------------------------

	// last_updated_in_object_matches_get_page verifies that the last_updated value
	// returned in a list_pages object equals what get_page returns for the same
	// page. The two tools must agree on a page's timestamp.
	t.Run("last_updated_in_object_matches_get_page", func(t *testing.T) {
		t.Parallel()
		c, _, _, _ := setupTestServerAndDir(t)

		callTool(t, c, "write_page", map[string]any{"page": "Consistency Ts Check", "content": "Content."})

		listResult := callTool(t, c, "list_pages", map[string]any{"sort_by": "newest"})
		var listResp listPagesTimestampResp
		parseJSON(t, listResult, &listResp)

		require.NotEmpty(t, listResp.Pages)
		require.Equal(t, "Consistency Ts Check", listResp.Pages[0].Page)
		listLastUpdated := listResp.Pages[0].LastUpdated

		getResult := callTool(t, c, "get_page", map[string]any{"page": "Consistency Ts Check"})
		var getResp getPageFullRespWithTS
		parseJSON(t, getResult, &getResp)

		assert.Equal(t, listLastUpdated, getResp.LastUpdated,
			"last_updated from list_pages must match last_updated from get_page for the same page")
	})

	// ---- unchanged formats for other sort modes ------------------------------

	// sort_alphabetical_returns_flat_strings verifies that the alphabetical sort
	// mode still returns pages as a flat array of strings, not objects. This
	// preserves backward compatibility and token efficiency for non-timestamp sorts.
	t.Run("sort_alphabetical_returns_flat_strings", func(t *testing.T) {
		t.Parallel()
		c, _, _, _ := setupTestServerAndDir(t)

		callTool(t, c, "write_page", map[string]any{"page": "Alpha Flat Check", "content": "Content."})
		callTool(t, c, "write_page", map[string]any{"page": "Beta Flat Check", "content": "Content."})

		result := callTool(t, c, "list_pages", map[string]any{"sort_by": "alphabetical"})

		// listPagesResp has Pages []string — parsing will fail if pages are objects.
		var resp listPagesResp
		parseJSON(t, result, &resp)

		require.Len(t, resp.Pages, 2)
		assert.Contains(t, resp.Pages, "Alpha Flat Check")
		assert.Contains(t, resp.Pages, "Beta Flat Check")
	})

	// sort_least_linked_returns_flat_strings verifies that least_linked retains
	// the flat-string format for backward compatibility.
	t.Run("sort_least_linked_returns_flat_strings", func(t *testing.T) {
		t.Parallel()
		c, _, _, _ := setupTestServerAndDir(t)

		callTool(t, c, "write_page", map[string]any{"page": "LL Flat Check", "content": "Content."})

		result := callTool(t, c, "list_pages", map[string]any{"sort_by": "least_linked"})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.Pages)
		assert.Contains(t, resp.Pages, "LL Flat Check")
	})

	// sort_most_linked_returns_flat_strings verifies that most_linked retains
	// the flat-string format for backward compatibility.
	t.Run("sort_most_linked_returns_flat_strings", func(t *testing.T) {
		t.Parallel()
		c, _, _, _ := setupTestServerAndDir(t)

		callTool(t, c, "write_page", map[string]any{"page": "ML Flat Check", "content": "Content."})

		result := callTool(t, c, "list_pages", map[string]any{"sort_by": "most_linked"})
		var resp listPagesResp
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.Pages)
		assert.Contains(t, resp.Pages, "ML Flat Check")
	})

	// ---- pagination ----------------------------------------------------------

	// pagination_works_with_newest_sort verifies that limit, offset, and total work
	// correctly when sort_by is newest. Results must not overlap across pages and
	// total must reflect the full count before pagination.
	t.Run("pagination_works_with_newest_sort", func(t *testing.T) {
		t.Parallel()
		c, _, _, dir := setupTestServerAndDir(t)

		// Create 5 pages with distinct mtimes so ordering is deterministic.
		for i := range 5 {
			name := fmt.Sprintf("Pagination Ts Page %d", i+1)
			callTool(t, c, "write_page", map[string]any{"page": name, "content": "Content."})
			setFileMtime(t, pageFilePath(dir, name),
				time.Date(2020+i, 1, 1, 0, 0, 0, 0, time.UTC))
		}

		// First window: limit 2, offset 0.
		result1 := callTool(t, c, "list_pages", map[string]any{
			"sort_by": "newest",
			"limit":   2,
			"offset":  0,
		})
		var resp1 listPagesTimestampResp
		parseJSON(t, result1, &resp1)

		assert.Equal(t, 5, resp1.Total, "total must reflect all 5 pages before pagination")
		require.Len(t, resp1.Pages, 2, "limit 2 must return exactly 2 pages")
		assert.Equal(t, 0, resp1.Offset)
		assert.Equal(t, 2, resp1.Limit)

		// Second window: limit 2, offset 2.
		result2 := callTool(t, c, "list_pages", map[string]any{
			"sort_by": "newest",
			"limit":   2,
			"offset":  2,
		})
		var resp2 listPagesTimestampResp
		parseJSON(t, result2, &resp2)

		require.Len(t, resp2.Pages, 2)
		assert.Equal(t, 2, resp2.Offset)

		// Pages must not overlap across pagination windows.
		window1 := make(map[string]bool)
		for _, p := range resp1.Pages {
			window1[p.Page] = true
		}
		for _, p := range resp2.Pages {
			assert.False(t, window1[p.Page],
				"pagination must not return the same page in two windows: %q appears in both", p.Page)
		}
	})

	// ---- content-modifying writes update the timestamp -----------------------

	// write_page_updates_last_updated verifies that calling write_page (a
	// content-modifying operation) advances the last_updated timestamp.
	t.Run("write_page_updates_last_updated", func(t *testing.T) {
		t.Parallel()
		c, _, _, dir := setupTestServerAndDir(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Write Updates Ts",
			"content": "Original content.",
		})

		// Pin the mtime to a known past time.
		pastTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		setFileMtime(t, pageFilePath(dir, "Write Updates Ts"), pastTime)

		// Write again — this is a content-modifying operation that must update the timestamp.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Write Updates Ts",
			"content": "Updated content.",
		})

		result := callTool(t, c, "get_page", map[string]any{"page": "Write Updates Ts"})
		var resp getPageFullRespWithTS
		parseJSON(t, result, &resp)

		ts, err := time.Parse(time.RFC3339, resp.LastUpdated)
		require.NoError(t, err)

		assert.True(t, ts.After(pastTime),
			"last_updated %q must be newer than the pinned past time after write_page", resp.LastUpdated)
	})

	// patch_page_updates_last_updated verifies that patch_page (a content-modifying
	// operation) also advances the last_updated timestamp.
	t.Run("patch_page_updates_last_updated", func(t *testing.T) {
		t.Parallel()
		c, _, _, dir := setupTestServerAndDir(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Patch Updates Ts",
			"content": "Original content.",
		})

		// Pin the mtime to a known past time.
		pastTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		setFileMtime(t, pageFilePath(dir, "Patch Updates Ts"), pastTime)

		// Patch the page — this is a content-modifying operation.
		callTool(t, c, "patch_page", map[string]any{
			"page": "Patch Updates Ts",
			"operations": []map[string]any{
				{"op": "replace", "old": "Original content.", "new": "Patched content."},
			},
		})

		result := callTool(t, c, "get_page", map[string]any{"page": "Patch Updates Ts"})
		var resp getPageFullRespWithTS
		parseJSON(t, result, &resp)

		ts, err := time.Parse(time.RFC3339, resp.LastUpdated)
		require.NoError(t, err)

		assert.True(t, ts.After(pastTime),
			"last_updated %q must be newer than the pinned past time after patch_page", resp.LastUpdated)
	})

	// rename_page_updates_last_updated verifies that rename_page advances
	// last_updated. Renaming rewrites the file's heading (# New Name), which is a
	// content write and must be reflected in the timestamp.
	t.Run("rename_page_updates_last_updated", func(t *testing.T) {
		t.Parallel()
		c, _, _, dir := setupTestServerAndDir(t) // non-git for deterministic mtime control

		callTool(t, c, "write_page", map[string]any{
			"page":    "Rename Updates Ts Src",
			"content": "Content for rename timestamp update test.",
		})

		// Pin the mtime to a specific past time so the rename's write is detectable.
		pastTime := time.Date(2021, 7, 4, 15, 30, 0, 0, time.UTC)
		setFileMtime(t, pageFilePath(dir, "Rename Updates Ts Src"), pastTime)

		callTool(t, c, "rename_page", map[string]any{
			"page":     "Rename Updates Ts Src",
			"new_name": "Rename Updates Ts Dst",
		})

		result := callTool(t, c, "get_page", map[string]any{"page": "Rename Updates Ts Dst"})
		var resp getPageFullRespWithTS
		parseJSON(t, result, &resp)

		require.NotEmpty(t, resp.LastUpdated)
		ts, err := time.Parse(time.RFC3339, resp.LastUpdated)
		require.NoError(t, err)

		assert.True(t, ts.After(pastTime),
			"rename_page rewrites the heading and must advance last_updated past the pinned time")
	})
}
