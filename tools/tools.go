package tools

import (
	"fmt"
	"os/exec"

	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/mark3labs/mcp-go/server"
)

// Register registers all memento MCP tools with the given server.
func Register(s *server.MCPServer, store *pages.Store, idx *index.Index) {
	registerWritePage(s, store, idx, nil)
	registerGetPage(s, store, idx)
	registerDeletePage(s, store, idx, nil)
	registerPatchPage(s, store, idx, nil)
	registerRenamePage(s, store, idx, nil)
	registerSearch(s, store, idx)
	registerListPages(s, store, idx)
}

// RegisterAutoCommit registers all memento MCP tools with auto-commit enabled.
// After every successful write operation, a git commit is created in contentDir.
func RegisterAutoCommit(s *server.MCPServer, store *pages.Store, idx *index.Index, contentDir string) {
	ac := &autoCommitter{dir: contentDir}
	registerWritePage(s, store, idx, ac)
	registerGetPage(s, store, idx)
	registerDeletePage(s, store, idx, ac)
	registerPatchPage(s, store, idx, ac)
	registerRenamePage(s, store, idx, ac)
	registerSearch(s, store, idx)
	registerListPages(s, store, idx)
}

// autoCommitter performs a git add + commit in a directory.
type autoCommitter struct {
	dir string
}

func (ac *autoCommitter) commit(message string, files []string) error {
	addArgs := append([]string{"add", "--"}, files...)
	addCmd := exec.Command("git", addArgs...)
	addCmd.Dir = ac.dir
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %w: %s", err, out)
	}

	commitCmd := exec.Command("git", "commit", "-m", message)
	commitCmd.Dir = ac.dir
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit failed: %w: %s", err, out)
	}
	return nil
}
