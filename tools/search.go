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
			Page        string `json:"page"`
			LastUpdated string `json:"last_updated,omitempty"`
			Snippet     string `json:"snippet"`
			Line        int    `json:"line"`
		}

		type resultEntry struct {
			Page        string   `json:"page"`
			Relevance   float64  `json:"relevance"`
			LastUpdated string   `json:"last_updated,omitempty"`
			Snippet     string   `json:"snippet"`
			Line        int      `json:"line"`
			LinkedPages []string `json:"linked_pages"`
		}

		// Separate direct BM25 matches from graph-boosted-only results.
		// Graph-boosted pages are treated as linked page details, not top-level results.
		directResults := rawResults[:0:0]
		for _, r := range rawResults {
			if r.IsDirect {
				directResults = append(directResults, r)
			}
		}

		// Collect the set of direct result page names (lowercased) for exclusion checks.
		directPageNames := make(map[string]bool, len(directResults))
		for _, r := range directResults {
			directPageNames[strings.ToLower(r.Page)] = true
		}

		linkedPageDetails := make([]linkedPageEntry, 0)
		// seenLinked tracks page names (lowercased) already added to linkedPageDetails.
		seenLinked := make(map[string]bool)

		// addLinkedDetail adds a page to linkedPageDetails if not already seen and
		// not already a direct result. Returns whether it was added.
		addLinkedDetail := func(name, snippet string, line int) bool {
			key := strings.ToLower(name)
			if seenLinked[key] || directPageNames[key] {
				return false
			}
			seenLinked[key] = true
			linkedPageDetails = append(linkedPageDetails, linkedPageEntry{
				Page:        name,
				LastUpdated: lastUpdatedForFile(store.FilePath(name)),
				Snippet:     snippet,
				Line:        line,
			})
			return true
		}

		results := make([]resultEntry, 0, len(directResults))
		tokenCount := 0

		for _, r := range directResults {
			relevance := 0.0
			if topScore > 0 {
				relevance = r.Score / topScore
			}

			// Build linked_pages string list from outbound wikilinks.
			linkedNames := idx.LinksTo(r.Page)
			linkedPageNames := make([]string, 0, len(linkedNames))
			newDetails := make([]linkedPageEntry, 0)
			collectDetail := func(name string) {
				lp, loadErr := store.Load(name)
				if loadErr != nil {
					return
				}
				key := strings.ToLower(lp.Name)
				if !seenLinked[key] && !directPageNames[key] {
					newDetails = append(newDetails, linkedPageEntry{
						Page:        lp.Name,
						LastUpdated: lastUpdatedForFile(store.FilePath(lp.Name)),
						Snippet:     firstBodyParagraph(lp.Body),
						Line:        2, // body starts at line 2 (line 1 is the heading)
					})
				}
			}
			for _, linkedName := range linkedNames {
				lp, loadErr := store.Load(linkedName)
				if loadErr != nil {
					// Broken link — skip.
					continue
				}
				linkedPageNames = append(linkedPageNames, lp.Name)
				collectDetail(linkedName)
			}
			// Also collect co-linked pages: pages that link TO this result and their other outbound links.
			for _, referrerName := range idx.LinkedFrom(r.Page) {
				for _, coLinkedName := range idx.LinksTo(referrerName) {
					collectDetail(coLinkedName)
				}
			}

			entry := resultEntry{
				Page:        r.Page,
				Relevance:   relevance,
				LastUpdated: lastUpdatedForFile(store.FilePath(r.Page)),
				Snippet:     r.Snippet,
				Line:        r.Line,
				LinkedPages: linkedPageNames,
			}

			// Apply token budget: count tokens for this entry plus any new linked page details.
			if maxTokens > 0 {
				entryJSON, marshalErr := json.Marshal(entry)
				if marshalErr == nil {
					entryTokens := len(strings.Fields(string(entryJSON)))
					detailTokens := 0
					for _, d := range newDetails {
						dJSON, dErr := json.Marshal(d)
						if dErr == nil {
							detailTokens += len(strings.Fields(string(dJSON)))
						}
					}
					if tokenCount+entryTokens+detailTokens > maxTokens {
						break
					}
					tokenCount += entryTokens + detailTokens
				}
			}

			// Commit new linked page details.
			for _, d := range newDetails {
				seenLinked[strings.ToLower(d.Page)] = true
				linkedPageDetails = append(linkedPageDetails, d)
			}
			results = append(results, entry)
		}

		// Mop up: add any graph-boosted (non-direct) results not already encountered
		// via outbound link traversal above.
		for _, r := range rawResults {
			if !r.IsDirect {
				addLinkedDetail(r.Page, r.Snippet, r.Line)
			}
		}

		resp := struct {
			Results           []resultEntry     `json:"results"`
			LinkedPageDetails []linkedPageEntry `json:"linked_page_details"`
		}{
			Results:           results,
			LinkedPageDetails: linkedPageDetails,
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
