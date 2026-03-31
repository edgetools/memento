package tools

import (
	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/mark3labs/mcp-go/server"
)

// Register registers all Memento MCP tools with the given server.
func Register(s *server.MCPServer, store *pages.Store, idx *index.Index) {
	registerWritePage(s, store, idx)
	registerGetPage(s, store, idx)
	registerDeletePage(s, store, idx)
}
