package tools_test

import (
	"context"
	"os/exec"
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

// ---- Auto-commit test helpers -----------------------------------------------

// initGitRepo initialises a bare git repo in dir with an initial empty commit
// so that `git log` works immediately after. Returns dir for convenience.
func initGitRepo(t *testing.T, dir string) string {
	t.Helper()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), out)
	}

	run("init")
	run("config", "--local", "user.email", "test@example.com")
	run("config", "--local", "user.name", "Test")
	// Create an initial commit so git log is valid from the start.
	run("commit", "--allow-empty", "-m", "init")

	return dir
}

// gitLog returns the one-line log messages for all commits in dir, most recent first.
func gitLog(t *testing.T, dir string) []string {
	t.Helper()
	cmd := exec.Command("git", "log", "--format=%s")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git log failed: %s", out)
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

// setupTestServerAutoCommit creates an isolated environment with a real git
// repo, a temp-dir-backed store, a fresh index, and a started+initialised
// in-process MCP client with all tools registered in auto-commit mode.
// The content directory is the git repo root.
func setupTestServerAutoCommit(t *testing.T) (*client.Client, string) {
	t.Helper()

	dir := t.TempDir()
	initGitRepo(t, dir)

	store := pages.NewStore(dir)
	idx := index.NewIndex()

	s := server.NewMCPServer("memento-test-autocommit", "0.0.0", server.WithToolCapabilities(true))
	tools.RegisterAutoCommit(s, store, idx, dir)

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

// commitCountAfterInit returns the number of commits beyond the initial "init"
// commit, i.e. the number of auto-commits created by tool calls.
func commitCountAfterInit(t *testing.T, dir string) int {
	t.Helper()
	logs := gitLog(t, dir)
	count := 0
	for _, msg := range logs {
		if msg != "init" {
			count++
		}
	}
	return count
}

// latestCommitMessage returns the subject line of the most recent commit.
func latestCommitMessage(t *testing.T, dir string) string {
	t.Helper()
	logs := gitLog(t, dir)
	require.NotEmpty(t, logs, "no commits found")
	return logs[0]
}

// ---- TestAutoCommit ---------------------------------------------------------

func TestAutoCommit(t *testing.T) {
	t.Parallel()

	// write_page creates a commit.
	t.Run("write_creates_commit", func(t *testing.T) {
		t.Parallel()
		c, dir := setupTestServerAutoCommit(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "The enchanter specialises in [[Crowd Control]].",
		})

		assert.Equal(t, 1, commitCountAfterInit(t, dir),
			"write_page should produce exactly one commit")

		msg := latestCommitMessage(t, dir)
		assert.Contains(t, msg, "Enchanter",
			"commit message should reference the page name")
		assert.Contains(t, msg, "memento:",
			"commit message should carry the 'memento:' prefix")
	})

	// delete_page creates a commit.
	t.Run("delete_creates_commit", func(t *testing.T) {
		t.Parallel()
		c, dir := setupTestServerAutoCommit(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Obsolete Concept",
			"content": "No longer needed.",
		})
		require.Equal(t, 1, commitCountAfterInit(t, dir))

		callTool(t, c, "delete_page", map[string]any{
			"page": "Obsolete Concept",
		})

		assert.Equal(t, 2, commitCountAfterInit(t, dir),
			"delete_page should produce exactly one additional commit")

		msg := latestCommitMessage(t, dir)
		assert.Contains(t, msg, "Obsolete Concept",
			"commit message should reference the deleted page name")
		assert.Contains(t, msg, "memento:",
			"commit message should carry the 'memento:' prefix")
	})

	// rename_page produces a single commit even when multiple files are updated.
	t.Run("rename_single_commit", func(t *testing.T) {
		t.Parallel()
		c, dir := setupTestServerAutoCommit(t)

		// Write the page to rename plus two pages that link to it.
		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "Abilities that restrict enemy movement.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Party Composition",
			"content": "[[Crowd Control]] is essential for difficult encounters.",
		})
		callTool(t, c, "write_page", map[string]any{
			"page":    "Dungeon Strategy",
			"content": "Assign [[Crowd Control]] duties before each pull.",
		})

		beforeRename := commitCountAfterInit(t, dir)

		callTool(t, c, "rename_page", map[string]any{
			"page":     "Crowd Control",
			"new_name": "Crowd Control Mechanics",
		})

		afterRename := commitCountAfterInit(t, dir)
		assert.Equal(t, 1, afterRename-beforeRename,
			"rename_page should produce exactly one commit regardless of how many pages were updated")

		msg := latestCommitMessage(t, dir)
		assert.Contains(t, msg, "memento:",
			"commit message should carry the 'memento:' prefix")
		assert.Contains(t, msg, "Crowd Control",
			"rename commit message should reference the old page name; got: %q", msg)
		assert.Contains(t, msg, "Crowd Control Mechanics",
			"rename commit message should reference the new page name; got: %q", msg)
	})

	// patch_page creates a commit.
	t.Run("patch_creates_commit", func(t *testing.T) {
		t.Parallel()
		c, dir := setupTestServerAutoCommit(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "Enchanter is the only CC class.",
		})
		require.Equal(t, 1, commitCountAfterInit(t, dir))

		callTool(t, c, "patch_page", map[string]any{
			"page": "Crowd Control",
			"operations": []any{
				map[string]any{
					"op":  "replace",
					"old": "Enchanter is the only CC class.",
					"new": "[[Enchanter]] is the primary CC class, though [[Bard]] has limited CC.",
				},
			},
		})

		assert.Equal(t, 2, commitCountAfterInit(t, dir),
			"patch_page should produce exactly one additional commit")

		msg := latestCommitMessage(t, dir)
		assert.Contains(t, msg, "Crowd Control",
			"commit message should reference the patched page name")
		assert.Contains(t, msg, "memento:",
			"commit message should carry the 'memento:' prefix")
	})

	// Without auto-commit, no git operations are performed.
	t.Run("disabled_no_commits", func(t *testing.T) {
		t.Parallel()

		// Use a git repo dir but register tools WITHOUT auto-commit (normal Register).
		dir := t.TempDir()
		initGitRepo(t, dir)

		store := pages.NewStore(dir)
		idx := index.NewIndex()

		s := server.NewMCPServer("memento-test-no-autocommit", "0.0.0", server.WithToolCapabilities(true))
		// Normal Register — no auto-commit.
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

		// Verify each write tool call succeeds (not vacuously passing due to silent failures).
		writeResult := callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "A utility class.",
		})
		require.False(t, writeResult.IsError, "write_page should succeed")

		patchResult := callTool(t, c, "patch_page", map[string]any{
			"page": "Enchanter",
			"operations": []any{
				map[string]any{
					"op":      "append",
					"content": "\n\nSpecialises in [[Mez]].",
				},
			},
		})
		require.False(t, patchResult.IsError, "patch_page should succeed")

		write2Result := callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "Abilities that restrict enemy movement.",
		})
		require.False(t, write2Result.IsError, "write_page (Crowd Control) should succeed")

		renameResult := callTool(t, c, "rename_page", map[string]any{
			"page":     "Crowd Control",
			"new_name": "Crowd Control Mechanics",
		})
		require.False(t, renameResult.IsError, "rename_page should succeed")

		deleteResult := callTool(t, c, "delete_page", map[string]any{
			"page": "Enchanter",
		})
		require.False(t, deleteResult.IsError, "delete_page should succeed")

		assert.Equal(t, 0, commitCountAfterInit(t, dir),
			"no commits should be created when auto-commit is disabled")
	})
}
