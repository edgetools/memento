package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type patchOp struct {
	Op      string
	Old     string
	New     string
	Lines   string
	Content string
}

func registerPatchPage(s *server.MCPServer, store *pages.Store, idx *index.Index, ac *autoCommitter) {
	tool := mcp.NewTool("patch_page",
		mcp.WithDescription("Performs targeted edits on an existing page. All operations in a call are applied atomically."),
		mcp.WithString("page",
			mcp.Required(),
			mcp.Description("The page name (case-insensitive)."),
		),
		mcp.WithArray("operations",
			mcp.Required(),
			mcp.Description(`List of patch operations. Each has an "op" field: "replace", "replace_lines", "append", or "prepend".`),
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

		opsVal, ok := args["operations"]
		if !ok || opsVal == nil {
			return mcp.NewToolResultError("operations is required"), nil
		}
		rawOps, ok := opsVal.([]any)
		if !ok {
			return mcp.NewToolResultError("operations must be an array"), nil
		}

		ops := make([]patchOp, 0, len(rawOps))
		for i, ro := range rawOps {
			m, ok := ro.(map[string]any)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("operation %d must be an object", i)), nil
			}
			var op patchOp
			op.Op, _ = m["op"].(string)
			op.Old, _ = m["old"].(string)
			op.New, _ = m["new"].(string)
			op.Lines, _ = m["lines"].(string)
			op.Content, _ = m["content"].(string)
			if op.Op == "" {
				return mcp.NewToolResultError(fmt.Sprintf("operation %d: 'op' field is required", i)), nil
			}
			ops = append(ops, op)
		}

		// Load the page, or create it on-demand for append/prepend operations.
		var p pages.Page
		if !store.Exists(pageName) {
			// Check whether all operations are create-capable (append/prepend).
			// If any existence-dependent operation (replace, replace_lines) is
			// present, fail atomically without creating the page.
			for i, op := range ops {
				switch op.Op {
				case "append", "prepend":
					// create-capable, ok
				case "replace", "replace_lines":
					return mcp.NewToolResultError(fmt.Sprintf("operation %d: page %q not found", i, pageName)), nil
				}
			}
			// All ops are create-capable: create the page with an empty body.
			created, err := store.Write(pageName, "")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			p = created
		} else {
			loaded, err := store.Load(pageName)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			p = loaded
		}

		// Build full content string for editing (heading + body).
		fullContent := "# " + p.Name + "\n" + p.Body
		contentLines := strings.Split(fullContent, "\n")
		totalLines := len(contentLines)

		// Validation pass: verify all operations against the original content.
		// If any operation is invalid, return an error before applying anything (atomicity).
		for i, op := range ops {
			switch op.Op {
			case "replace":
				if op.Old == "" {
					return mcp.NewToolResultError(fmt.Sprintf("operation %d: 'old' is required for replace", i)), nil
				}
				count := strings.Count(fullContent, op.Old)
				if count == 0 {
					return mcp.NewToolResultError(fmt.Sprintf("operation %d: text not found: %q", i, op.Old)), nil
				}
				if count > 1 {
					return mcp.NewToolResultError(fmt.Sprintf("operation %d: text is ambiguous (appears %d times): %q", i, count, op.Old)), nil
				}
			case "replace_lines":
				if op.Lines == "" {
					return mcp.NewToolResultError(fmt.Sprintf("operation %d: 'lines' is required for replace_lines", i)), nil
				}
				start, end, parseErr := parseLineRange(op.Lines)
				if parseErr != nil {
					return mcp.NewToolResultError(fmt.Sprintf("operation %d: invalid lines range %q: %v", i, op.Lines, parseErr)), nil
				}
				if start < 1 || end > totalLines || start > end {
					return mcp.NewToolResultError(fmt.Sprintf("operation %d: line range %q out of bounds (page has %d lines)", i, op.Lines, totalLines)), nil
				}
			case "append", "prepend":
				// always valid
			default:
				return mcp.NewToolResultError(fmt.Sprintf("operation %d: unknown op %q", i, op.Op)), nil
			}
		}

		// Apply pass: work on a copy for atomicity.
		// line_numbers_pre_op: all replace_lines operations reference original line numbers.
		// We track a cumulative lineOffset to map original line numbers to current positions.
		content := fullContent
		lineOffset := 0

		for _, op := range ops {
			switch op.Op {
			case "replace":
				oldNL := strings.Count(op.Old, "\n")
				newNL := strings.Count(op.New, "\n")
				content = strings.Replace(content, op.Old, op.New, 1)
				lineOffset += newNL - oldNL

			case "replace_lines":
				start, end, _ := parseLineRange(op.Lines)
				adjStart := start + lineOffset
				adjEnd := end + lineOffset
				lines := strings.Split(content, "\n")
				var newLines []string
				if op.New != "" {
					newLines = strings.Split(op.New, "\n")
				}
				updated := make([]string, 0, len(lines)-int(adjEnd-adjStart+1)+len(newLines))
				updated = append(updated, lines[:adjStart-1]...)
				updated = append(updated, newLines...)
				updated = append(updated, lines[adjEnd:]...)
				content = strings.Join(updated, "\n")
				lineOffset += len(newLines) - (end - start + 1)

			case "append":
				content += op.Content

			case "prepend":
				// Insert content after the first line (the managed heading).
				nlIdx := strings.Index(content, "\n")
				if nlIdx >= 0 {
					content = content[:nlIdx+1] + op.Content + content[nlIdx+1:]
				} else {
					content += "\n" + op.Content
				}
			}
		}

		// Strip the managed heading before writing back (store.Write re-adds it).
		body := content
		if strings.HasPrefix(body, "# ") {
			nlIdx := strings.Index(body, "\n")
			if nlIdx >= 0 {
				body = body[nlIdx+1:]
			} else {
				body = ""
			}
		}

		updatedPage, err := store.Write(p.Name, body)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		idx.Add(updatedPage)

		linksTo := updatedPage.WikiLinks
		if linksTo == nil {
			linksTo = []string{}
		}

		resp := struct {
			Page           string   `json:"page"`
			LinksTo        []string `json:"links_to"`
			CommitFailures []string `json:"commit_failures,omitempty"`
		}{
			Page:    updatedPage.Name,
			LinksTo: linksTo,
		}

		if ac != nil {
			if commitErr := ac.commit(fmt.Sprintf("memento: patched %q", updatedPage.Name), []string{store.FilePath(updatedPage.Name)}); commitErr != nil {
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
