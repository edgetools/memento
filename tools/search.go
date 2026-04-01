package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSearch(s *server.MCPServer, store *pages.Store, idx *index.Index) {
	tool := mcp.NewTool("search",
		mcp.WithDescription("Queries the brain and returns relevance-ranked results with contextual snippets and graph-connected pages."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The search query."),
		),
		mcp.WithNumber("max_results",
			mcp.Description("Maximum number of results to return (default: 10)."),
		),
		mcp.WithNumber("max_tokens",
			mcp.Description("Approximate token budget for the response. Results are added until the budget is reached."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()

		queryVal, ok := args["query"]
		if !ok || queryVal == nil {
			return mcp.NewToolResultError("query is required"), nil
		}
		query, _ := queryVal.(string)
		if query == "" {
			return mcp.NewToolResultError("query must not be empty"), nil
		}

		maxResults := 10
		if v, ok := args["max_results"]; ok && v != nil {
			if f, ok := v.(float64); ok && f > 0 {
				maxResults = int(f)
			}
		}

		maxTokens := 0
		if v, ok := args["max_tokens"]; ok && v != nil {
			if f, ok := v.(float64); ok && f > 0 {
				maxTokens = int(f)
			}
		}

		rawResults := idx.Search(query, maxResults)

		// Normalize relevance scores relative to the top result.
		topScore := 0.0
		if len(rawResults) > 0 {
			topScore = rawResults[0].Score
		}

		type linkedPageEntry struct {
			Page    string `json:"page"`
			Snippet string `json:"snippet"`
			Line    int    `json:"line"`
		}

		type resultEntry struct {
			Page        string            `json:"page"`
			Relevance   float64           `json:"relevance"`
			Snippet     string            `json:"snippet"`
			Line        int               `json:"line"`
			LinkedPages []linkedPageEntry `json:"linked_pages"`
		}

		results := make([]resultEntry, 0, len(rawResults))
		tokenCount := 0

		for _, r := range rawResults {
			relevance := 0.0
			if topScore > 0 {
				relevance = r.Score / topScore
			}

			// Build linked_pages from outbound wikilinks.
			linkedNames := idx.LinksTo(r.Page)
			linkedPages := make([]linkedPageEntry, 0, len(linkedNames))
			for _, linkedName := range linkedNames {
				lp, loadErr := store.Load(linkedName)
				if loadErr != nil {
					// Broken link — skip.
					continue
				}
				snippet := firstBodyParagraph(lp.Body)
				linkedPages = append(linkedPages, linkedPageEntry{
					Page:    lp.Name,
					Snippet: snippet,
					Line:    2, // body starts at line 2 (line 1 is the heading)
				})
			}

			entry := resultEntry{
				Page:        r.Page,
				Relevance:   relevance,
				Snippet:     r.Snippet,
				Line:        r.Line,
				LinkedPages: linkedPages,
			}

			// Apply token budget: count tokens in this entry's JSON representation.
			if maxTokens > 0 {
				entryJSON, marshalErr := json.Marshal(entry)
				if marshalErr == nil {
					entryTokens := len(strings.Fields(string(entryJSON)))
					if tokenCount+entryTokens > maxTokens {
						break
					}
					tokenCount += entryTokens
				}
			}

			results = append(results, entry)
		}

		resp := struct {
			Results []resultEntry `json:"results"`
		}{
			Results: results,
		}

		data, err := json.Marshal(resp)
		if err != nil {
			return mcp.NewToolResultError("internal error marshaling response: " + err.Error()), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})
}

// firstBodyParagraph returns the first paragraph of the page body, truncated to ~300 chars.
func firstBodyParagraph(body string) string {
	body = strings.TrimSpace(body)
	if idx := strings.Index(body, "\n\n"); idx >= 0 {
		body = body[:idx]
	}
	body = strings.TrimSpace(body)
	if len(body) > 300 {
		body = body[:300]
	}
	return body
}
