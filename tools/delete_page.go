package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerDeletePage(s *server.MCPServer, store *pages.Store, idx *index.Index, ac *autoCommitter) {
	tool := mcp.NewTool("delete_page",
		mcp.WithDescription("Removes a page from the brain."),
		mcp.WithString("page",
			mcp.Required(),
			mcp.Description("The page name to delete (case-insensitive)."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()

		pageVal, pagePresent := args["page"]
		if !pagePresent || pageVal == nil {
			return mcp.NewToolResultError("page is required"), nil
		}
		pageName, _ := pageVal.(string)
		if pageName == "" {
			return mcp.NewToolResultError("page is required"), nil
		}

		// Load the page first to get the canonical name before deletion.
		p, err := store.Load(pageName)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		canonicalName := p.Name
		pageFilePath := store.FilePath(canonicalName)

		if err := store.Delete(pageName); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		idx.Remove(canonicalName)

		resp := struct {
			Page           string   `json:"page"`
			CommitFailures []string `json:"commit_failures,omitempty"`
		}{
			Page: canonicalName,
		}

		if ac != nil {
			if commitErr := ac.commit(fmt.Sprintf("memento: deleted %q", canonicalName), []string{pageFilePath}); commitErr != nil {
				resp.CommitFailures = append(resp.CommitFailures, commitErr.Error())
			}
		}

		data, err := json.Marshal(resp)
		if err != nil {
			return mcp.NewToolResultError("internal error marshaling response: " + err.Error()), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})
}
