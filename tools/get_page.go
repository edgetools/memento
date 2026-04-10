package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerGetPage(s *server.MCPServer, store *pages.Store, idx *index.Index) {
	tool := mcp.NewTool("get_page",
		mcp.WithDescription("Returns the full markdown content of a page, or specific line ranges."),
		mcp.WithString("page",
			mcp.Required(),
			mcp.Description("The page name (case-insensitive)."),
		),
		mcp.WithArray("lines",
			mcp.Description(`Optional line ranges to fetch (e.g. ["10-25", "34", "52-68"]). 1-indexed, inclusive.`),
			mcp.WithStringItems(),
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

		p, err := store.Load(pageName)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Build full content string (heading + body).
		fullContent := "# " + p.Name + "\n" + p.Body
		contentLines := strings.Split(fullContent, "\n")
		totalLines := len(contentLines)

		linksTo := idx.LinksTo(p.Name)
		if linksTo == nil {
			linksTo = []string{}
		}
		linkedFrom := idx.LinkedFrom(p.Name)
		if linkedFrom == nil {
			linkedFrom = []string{}
		}

		// Check if line ranges were requested.
		linesVal := args["lines"]
		if linesVal == nil {
			// Full page response.
			resp := struct {
				Page        string   `json:"page"`
				Content     string   `json:"content"`
				TotalLines  int      `json:"total_lines"`
				LastUpdated string   `json:"last_updated,omitempty"`
				LinksTo     []string `json:"links_to"`
				LinkedFrom  []string `json:"linked_from"`
			}{
				Page:        p.Name,
				Content:     fullContent,
				TotalLines:  totalLines,
				LastUpdated: lastUpdatedForFile(store.FilePath(p.Name)),
				LinksTo:     linksTo,
				LinkedFrom:  linkedFrom,
			}
			data, err := json.Marshal(resp)
			if err != nil {
				return mcp.NewToolResultError("internal error marshaling response: " + err.Error()), nil
			}
			return mcp.NewToolResultText(string(data)), nil
		}

		// Parse line ranges.
		rawRanges, ok := linesVal.([]any)
		if !ok {
			return mcp.NewToolResultError("lines must be an array of strings"), nil
		}

		type section struct {
			Lines   string `json:"lines"`
			Content string `json:"content"`
		}
		sections := make([]section, 0, len(rawRanges))

		for _, rv := range rawRanges {
			rangeStr, _ := rv.(string)
			if rangeStr == "" {
				return mcp.NewToolResultError("line range must be a non-empty string"), nil
			}

			start, end, parseErr := parseLineRange(rangeStr)
			if parseErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid line range %q: %v", rangeStr, parseErr)), nil
			}

			if start < 1 || end > totalLines || start > end {
				return mcp.NewToolResultError(fmt.Sprintf(
					"line range %q out of bounds: page has %d lines", rangeStr, totalLines,
				)), nil
			}

			// Extract lines: 1-indexed inclusive → 0-indexed slice [start-1 : end].
			sectionContent := strings.Join(contentLines[start-1:end], "\n")
			sections = append(sections, section{Lines: rangeStr, Content: sectionContent})
		}

		resp := struct {
			Page        string    `json:"page"`
			Sections    []section `json:"sections"`
			TotalLines  int       `json:"total_lines"`
			LastUpdated string    `json:"last_updated,omitempty"`
			LinksTo     []string  `json:"links_to"`
			LinkedFrom  []string  `json:"linked_from"`
		}{
			Page:        p.Name,
			Sections:    sections,
			TotalLines:  totalLines,
			LastUpdated: lastUpdatedForFile(store.FilePath(p.Name)),
			LinksTo:     linksTo,
			LinkedFrom:  linkedFrom,
		}
		data, err := json.Marshal(resp)
		if err != nil {
			return mcp.NewToolResultError("internal error marshaling response: " + err.Error()), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})
}

// parseLineRange parses a range string like "10-25" or "10" into (start, end).
// Both start and end are 1-indexed and inclusive.
func parseLineRange(s string) (start, end int, err error) {
	if idx := strings.Index(s, "-"); idx >= 0 {
		startStr := s[:idx]
		endStr := s[idx+1:]
		start, err = strconv.Atoi(startStr)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid start: %w", err)
		}
		end, err = strconv.Atoi(endStr)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid end: %w", err)
		}
	} else {
		start, err = strconv.Atoi(s)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid line number: %w", err)
		}
		end = start
	}
	return start, end, nil
}
