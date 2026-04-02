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

func registerWritePage(s *server.MCPServer, store *pages.Store, idx *index.Index, ac *autoCommitter) {
	tool := mcp.NewTool("write_page",
		mcp.WithDescription("Creates a new page or fully replaces an existing page's content."),
		mcp.WithString("page",
			mcp.Required(),
			mcp.Description("The page name used as the canonical identifier."),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("The page body content (markdown). A heading will be managed automatically."),
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

		contentVal, contentPresent := args["content"]
		if !contentPresent {
			return mcp.NewToolResultError("content is required"), nil
		}
		content := ""
		if contentVal != nil {
			content, _ = contentVal.(string)
		}

		p, err := store.Write(pageName, content)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		idx.Add(p)

		linksTo := p.WikiLinks
		if linksTo == nil {
			linksTo = []string{}
		}

		resp := struct {
			Page           string   `json:"page"`
			LinksTo        []string `json:"links_to"`
			CommitFailures []string `json:"commit_failures,omitempty"`
		}{
			Page:    p.Name,
			LinksTo: linksTo,
		}

		if ac != nil {
			if commitErr := ac.commit(fmt.Sprintf("memento: updated %q", p.Name)); commitErr != nil {
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
