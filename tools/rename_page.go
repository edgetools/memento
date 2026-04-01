package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerRenamePage(s *server.MCPServer, store *pages.Store, idx *index.Index, ac *autoCommitter) {
	tool := mcp.NewTool("rename_page",
		mcp.WithDescription("Renames a page and updates all [[wikilinks]] across the brain that reference the old name."),
		mcp.WithString("page",
			mcp.Required(),
			mcp.Description("The current page name (case-insensitive)."),
		),
		mcp.WithString("new_name",
			mcp.Required(),
			mcp.Description("The new page name."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()

		pageVal, ok := args["page"]
		if !ok || pageVal == nil {
			return mcp.NewToolResultError("page is required"), nil
		}
		pageName, _ := pageVal.(string)
		if pageName == "" {
			return mcp.NewToolResultError("page is required"), nil
		}

		newNameVal, ok := args["new_name"]
		if !ok || newNameVal == nil {
			return mcp.NewToolResultError("new_name is required"), nil
		}
		newName, _ := newNameVal.(string)
		if newName == "" {
			return mcp.NewToolResultError("new_name is required"), nil
		}

		// Load source page to get canonical name.
		srcPage, err := store.Load(pageName)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		oldName := srcPage.Name

		// Rename in the store (updates filename and heading).
		if err := store.Rename(oldName, newName); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Remove old index entry.
		idx.Remove(oldName)

		// Scan all pages and replace [[OldName]] wikilinks with [[NewName]].
		// Also add the renamed page and any updated pages to the index.
		pattern := regexp.MustCompile(`(?i)\[\[` + regexp.QuoteMeta(oldName) + `\]\]`)
		replacement := "[[" + newName + "]]"

		for _, p := range store.Scan() {
			newBody := pattern.ReplaceAllLiteralString(p.Body, replacement)
			if newBody != p.Body {
				// Wikilinks were updated — write back and re-index.
				updated, writeErr := store.Write(p.Name, newBody)
				if writeErr == nil {
					idx.Add(updated)
				}
			} else if pages.NamesMatch(p.Name, newName) {
				// This is the renamed page with no self-referential links — add to index.
				idx.Add(p)
			}
		}

		resp := struct {
			Page    string `json:"page"`
			OldName string `json:"old_name"`
		}{
			Page:    newName,
			OldName: oldName,
		}
		data, err := json.Marshal(resp)
		if err != nil {
			return mcp.NewToolResultError("internal error marshaling response: " + err.Error()), nil
		}
		if ac != nil {
			_ = ac.commit(fmt.Sprintf("memento: renamed %q to %q", oldName, newName))
		}
		return mcp.NewToolResultText(string(data)), nil
	})
}
