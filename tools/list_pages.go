package tools

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerListPages(s *server.MCPServer, store *pages.Store, idx *index.Index) {
	tool := mcp.NewTool("list_pages",
		mcp.WithDescription("Returns a paginated, sorted list of page names with no content — names only."),
		mcp.WithString("sort_by",
			mcp.Description(`Sort order: "alphabetical" (default), "least_linked", or "most_linked".`),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of page names to return (default: 50)."),
		),
		mcp.WithNumber("offset",
			mcp.Description("Number of pages to skip before returning results (default: 0)."),
		),
		mcp.WithArray("filter",
			mcp.Description("Only pages whose names contain ALL of the given keywords are included (case-insensitive substring match, AND semantics)."),
			mcp.Items(map[string]any{"type": "string"}),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()

		// Parse sort_by (default: alphabetical).
		sortBy := "alphabetical"
		if v, ok := args["sort_by"]; ok && v != nil {
			if sv, ok := v.(string); ok && sv != "" {
				sortBy = sv
			}
		}

		// Parse limit (default: 50).
		limit := 50
		if v, ok := args["limit"]; ok && v != nil {
			if f, ok := v.(float64); ok && f > 0 {
				limit = int(f)
			}
		}

		// Parse offset (default: 0).
		offset := 0
		if v, ok := args["offset"]; ok && v != nil {
			if f, ok := v.(float64); ok && f >= 0 {
				offset = int(f)
			}
		}

		// Parse filter keywords (lowercased for case-insensitive matching).
		var filterKeywords []string
		if v, ok := args["filter"]; ok && v != nil {
			if arr, ok := v.([]any); ok {
				for _, item := range arr {
					if kw, ok := item.(string); ok && kw != "" {
						filterKeywords = append(filterKeywords, strings.ToLower(kw))
					}
				}
			}
		}

		// Scan all pages and apply filter.
		allPages := store.Scan()
		filtered := make([]pages.Page, 0, len(allPages))
		for _, p := range allPages {
			if listPagesMatchesFilter(p.Name, filterKeywords) {
				filtered = append(filtered, p)
			}
		}

		// Sort.
		switch sortBy {
		case "most_linked":
			sort.SliceStable(filtered, func(i, j int) bool {
				ci := len(idx.LinkedFrom(filtered[i].Name))
				cj := len(idx.LinkedFrom(filtered[j].Name))
				if ci != cj {
					return ci > cj
				}
				return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
			})
		case "least_linked":
			sort.SliceStable(filtered, func(i, j int) bool {
				ci := len(idx.LinkedFrom(filtered[i].Name))
				cj := len(idx.LinkedFrom(filtered[j].Name))
				if ci != cj {
					return ci < cj
				}
				return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
			})
		default: // alphabetical
			sort.SliceStable(filtered, func(i, j int) bool {
				return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
			})
		}

		total := len(filtered)

		// Apply pagination.
		start := min(offset, total)
		end := min(start+limit, total)
		pageSlice := filtered[start:end]

		// Build names-only slice (never nil so it marshals as []).
		names := make([]string, len(pageSlice))
		for i, p := range pageSlice {
			names[i] = p.Name
		}

		resp := struct {
			Pages  []string `json:"pages"`
			Total  int      `json:"total"`
			Offset int      `json:"offset"`
			Limit  int      `json:"limit"`
		}{
			Pages:  names,
			Total:  total,
			Offset: offset,
			Limit:  limit,
		}

		data, err := json.Marshal(resp)
		if err != nil {
			return mcp.NewToolResultError("internal error marshaling response: " + err.Error()), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})
}

// listPagesMatchesFilter returns true if name contains every keyword in
// keywords as a case-insensitive substring. An empty keywords slice matches
// everything.
func listPagesMatchesFilter(name string, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	lower := strings.ToLower(name)
	for _, kw := range keywords {
		if !strings.Contains(lower, kw) {
			return false
		}
	}
	return true
}
