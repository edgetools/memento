package tools_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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

// ---- Selective-staging test helpers -----------------------------------------

// setupTestServerAutoCommitSubdir creates a git repo with contentDir as a
// subdirectory (not the repo root) — the common real-world setup where a
// memento brain lives inside a larger repository. Unrelated files can be
// planted in repoDir alongside contentDir to verify they are never staged by
// memento's auto-commit logic.
//
// Returns the started+initialised in-process MCP client, the repo root, and
// the content directory path.
func setupTestServerAutoCommitSubdir(t *testing.T) (*client.Client, string, string) {
	t.Helper()

	repoDir := t.TempDir()
	initGitRepo(t, repoDir)

	// contentDir is a subdirectory — not the repo root.
	contentDir := filepath.Join(repoDir, "content")
	require.NoError(t, os.MkdirAll(contentDir, 0o755))

	store := pages.NewStore(contentDir)
	idx := index.NewIndex()

	s := server.NewMCPServer("memento-test-subdir-autocommit", "0.0.0", server.WithToolCapabilities(true))
	tools.RegisterAutoCommit(s, store, idx, contentDir)

	c, err := client.NewInProcessClient(s)
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })

	require.NoError(t, c.Start(context.Background()))

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "memento-test-client", Version: "0.0.0"}
	_, err = c.Initialize(context.Background(), initReq)
	require.NoError(t, err)

	return c, repoDir, contentDir
}

// plantUnrelatedDirtyFile writes an arbitrary file directly in repoDir (not
// inside contentDir) to simulate unrelated pending changes in the repository.
func plantUnrelatedDirtyFile(t *testing.T, repoDir, name string) {
	t.Helper()
	path := filepath.Join(repoDir, name)
	require.NoError(t, os.WriteFile(path, []byte("unrelated content — must never be staged by memento"), 0o644))
}

// gitFilesInLatestCommit returns the file paths (relative to repoDir) that
// were touched in the most recent commit.
func gitFilesInLatestCommit(t *testing.T, repoDir string) []string {
	t.Helper()
	cmd := exec.Command("git", "diff-tree", "--no-commit-id", "-r", "--name-only", "HEAD")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git diff-tree failed: %s", out)
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

// gitStatusShort returns the raw output of `git status --short` for repoDir.
// Untracked files appear as "?? <path>", modified-but-unstaged as " M <path>".
func gitStatusShort(t *testing.T, repoDir string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--short")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git status --short failed: %s", out)
	return strings.TrimSpace(string(out))
}

// ---- TestAutoCommitSelectiveStaging -----------------------------------------

// TestAutoCommitSelectiveStaging verifies that when -auto-commit is enabled and
// contentDir is a subdirectory of a larger git repo, each write tool stages ONLY
// the specific files it modified — never unrelated dirty files that happen to
// live elsewhere in the repository.
//
// This is the primary regression-test suite for the git-add-selective-files fix
// (previously the implementation used `git add -A`, which swept all repo changes
// into every memento commit).
func TestAutoCommitSelectiveStaging(t *testing.T) {
	t.Parallel()

	// write_page: only the written page file is committed; unrelated files stay dirty.
	t.Run("write_page_stages_only_its_file", func(t *testing.T) {
		t.Parallel()
		c, repoDir, _ := setupTestServerAutoCommitSubdir(t)

		// Plant an unrelated dirty file in the repo root before the write.
		plantUnrelatedDirtyFile(t, repoDir, "unrelated.txt")

		callTool(t, c, "write_page", map[string]any{
			"page":    "Enchanter",
			"content": "The enchanter specialises in [[Crowd Control]].",
		})

		committed := gitFilesInLatestCommit(t, repoDir)

		// The committed set must contain the written page.
		committedStr := strings.Join(committed, "\n")
		assert.Contains(t, committedStr, "Enchanter.md",
			"write_page: committed files must include the written page file")

		// The unrelated file must NOT appear in the commit.
		assert.NotContains(t, committedStr, "unrelated.txt",
			"write_page: unrelated dirty file must not be staged in auto-commit")

		// The unrelated file must still show up as dirty in the working tree.
		status := gitStatusShort(t, repoDir)
		assert.Contains(t, status, "unrelated.txt",
			"write_page: unrelated dirty file must remain in the working tree unstaged")
	})

	// patch_page: only the patched page file is committed; unrelated files stay dirty.
	t.Run("patch_page_stages_only_its_file", func(t *testing.T) {
		t.Parallel()
		c, repoDir, _ := setupTestServerAutoCommitSubdir(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Crowd Control",
			"content": "Enchanter is the primary CC class.",
		})

		// Plant the unrelated file after the setup write so it is not part of
		// any prior commit.
		plantUnrelatedDirtyFile(t, repoDir, "unrelated.txt")

		callTool(t, c, "patch_page", map[string]any{
			"page": "Crowd Control",
			"operations": []any{
				map[string]any{
					"op":      "append",
					"content": "\n\nBards also have limited CC.",
				},
			},
		})

		committed := gitFilesInLatestCommit(t, repoDir)
		committedStr := strings.Join(committed, "\n")

		assert.Contains(t, committedStr, "Crowd Control.md",
			"patch_page: committed files must include the patched page file")
		assert.NotContains(t, committedStr, "unrelated.txt",
			"patch_page: unrelated dirty file must not be staged in auto-commit")

		status := gitStatusShort(t, repoDir)
		assert.Contains(t, status, "unrelated.txt",
			"patch_page: unrelated dirty file must remain in the working tree unstaged")
	})

	// delete_page: only the deleted page file is committed; unrelated files stay dirty.
	t.Run("delete_page_stages_only_its_file", func(t *testing.T) {
		t.Parallel()
		c, repoDir, _ := setupTestServerAutoCommitSubdir(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Obsolete Concept",
			"content": "No longer needed.",
		})

		// Plant the unrelated file after the setup write.
		plantUnrelatedDirtyFile(t, repoDir, "unrelated.txt")

		callTool(t, c, "delete_page", map[string]any{
			"page": "Obsolete Concept",
		})

		committed := gitFilesInLatestCommit(t, repoDir)
		committedStr := strings.Join(committed, "\n")

		assert.Contains(t, committedStr, "Obsolete Concept.md",
			"delete_page: committed files must include the deleted page file")
		assert.NotContains(t, committedStr, "unrelated.txt",
			"delete_page: unrelated dirty file must not be staged in auto-commit")

		status := gitStatusShort(t, repoDir)
		assert.Contains(t, status, "unrelated.txt",
			"delete_page: unrelated dirty file must remain in the working tree unstaged")
	})

	// rename_page: the renamed page and all rewritten linker pages are committed
	// together in a single commit; unrelated files stay dirty.
	t.Run("rename_page_stages_renamed_and_linkers_only", func(t *testing.T) {
		t.Parallel()
		c, repoDir, _ := setupTestServerAutoCommitSubdir(t)

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

		// Plant the unrelated file after all setup writes so it is not part of
		// any prior commit.
		plantUnrelatedDirtyFile(t, repoDir, "unrelated.txt")

		callTool(t, c, "rename_page", map[string]any{
			"page":     "Crowd Control",
			"new_name": "Crowd Control Mechanics",
		})

		committed := gitFilesInLatestCommit(t, repoDir)
		committedStr := strings.Join(committed, "\n")

		// The OLD filename must appear in the commit as a staged deletion.
		assert.Contains(t, committedStr, "Crowd Control.md",
			"rename_page: old page filename must be staged (as a deletion) in the commit")

		// The new filename for the renamed page must appear in the commit.
		assert.Contains(t, committedStr, "Crowd Control Mechanics.md",
			"rename_page: new page filename must be staged in the commit")

		// Both linker pages must appear in the commit.
		assert.Contains(t, committedStr, "Party Composition.md",
			"rename_page: linker page Party Composition.md must be staged")
		assert.Contains(t, committedStr, "Dungeon Strategy.md",
			"rename_page: linker page Dungeon Strategy.md must be staged")

		// The unrelated file must NOT appear in the commit.
		assert.NotContains(t, committedStr, "unrelated.txt",
			"rename_page: unrelated dirty file must not be staged in auto-commit")

		// The unrelated file must still be dirty in the working tree.
		status := gitStatusShort(t, repoDir)
		assert.Contains(t, status, "unrelated.txt",
			"rename_page: unrelated dirty file must remain in the working tree unstaged")
	})

	// rename_page with no linkers: only the renamed page file itself is committed.
	t.Run("rename_page_no_linkers_stages_only_renamed_file", func(t *testing.T) {
		t.Parallel()
		c, repoDir, _ := setupTestServerAutoCommitSubdir(t)

		callTool(t, c, "write_page", map[string]any{
			"page":    "Orphan Page",
			"content": "This page has no incoming wikilinks.",
		})

		plantUnrelatedDirtyFile(t, repoDir, "unrelated.txt")

		callTool(t, c, "rename_page", map[string]any{
			"page":     "Orphan Page",
			"new_name": "Orphan Page Renamed",
		})

		committed := gitFilesInLatestCommit(t, repoDir)
		committedStr := strings.Join(committed, "\n")

		// The OLD filename must appear in the commit as a staged deletion.
		assert.Contains(t, committedStr, "Orphan Page.md",
			"rename_page (no linkers): old page filename must be staged (as a deletion) in the commit")

		// The new filename must appear in the commit.
		assert.Contains(t, committedStr, "Orphan Page Renamed.md",
			"rename_page (no linkers): new page filename must be staged")

		// The unrelated file must NOT appear in the commit.
		assert.NotContains(t, committedStr, "unrelated.txt",
			"rename_page (no linkers): unrelated dirty file must not be staged in auto-commit")

		// The unrelated file must still be dirty.
		status := gitStatusShort(t, repoDir)
		assert.Contains(t, status, "unrelated.txt",
			"rename_page (no linkers): unrelated dirty file must remain in the working tree unstaged")
	})
}

// TestAutoCommitSelectiveStagingMultipleUnrelated verifies that even when
// multiple unrelated dirty files are present in the repository alongside the
// content directory, none of them are pulled into a memento auto-commit.
func TestAutoCommitSelectiveStagingMultipleUnrelated(t *testing.T) {
	t.Parallel()

	c, repoDir, _ := setupTestServerAutoCommitSubdir(t)

	// Plant several unrelated dirty files in the repo root.
	for _, name := range []string{"notes.txt", "draft.md", "scratch.go"} {
		plantUnrelatedDirtyFile(t, repoDir, name)
	}

	callTool(t, c, "write_page", map[string]any{
		"page":    "Enchanter",
		"content": "The enchanter is a utility class.",
	})

	committed := gitFilesInLatestCommit(t, repoDir)
	committedStr := strings.Join(committed, "\n")

	assert.Contains(t, committedStr, "Enchanter.md",
		"the written page must appear in the auto-commit")

	for _, name := range []string{"notes.txt", "draft.md", "scratch.go"} {
		assert.NotContains(t, committedStr, name,
			"unrelated file %q must not be staged in memento's auto-commit", name)
	}

	// All unrelated files must still be dirty.
	status := gitStatusShort(t, repoDir)
	for _, name := range []string{"notes.txt", "draft.md", "scratch.go"} {
		assert.Contains(t, status, name,
			"unrelated file %q must remain unstaged in the working tree", name)
	}
}
