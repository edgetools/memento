package pages

import (
	"regexp"
	"strings"
)

// Page holds the parsed representation of a markdown page.
type Page struct {
	// Name is the page name as given to Parse (the lookup key).
	Name string
	// Title is the text of the first H1 heading, or Name if no H1 is present.
	Title string
	// Body is the page content with the H1 heading line removed.
	Body string
	// WikiLinks contains unique wikilink targets extracted from the page,
	// in the original casing they appear in the source. Links inside code
	// spans or fenced code blocks are excluded.
	WikiLinks []string
	// Lines is the total number of lines in the original content.
	Lines int
}

var (
	h1RE       = regexp.MustCompile(`^# (.+)`)
	wikilinkRE = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)
)

// Parse parses markdown content for a page identified by name.
// It extracts the title, body, wikilinks, and line count.
func Parse(name string, content []byte) Page {
	raw := string(content)
	lines := splitLines(raw)

	p := Page{
		Name:  name,
		Title: name,
		Lines: len(lines),
	}

	// Find H1 title (first line only for simplicity, but scan all lines).
	titleLineIdx := -1
	for i, line := range lines {
		if m := h1RE.FindStringSubmatch(line); m != nil {
			p.Title = m[1]
			titleLineIdx = i
			break
		}
	}

	// Build body: all lines except the H1 line.
	var bodyLines []string
	for i, line := range lines {
		if i == titleLineIdx {
			continue
		}
		bodyLines = append(bodyLines, line)
	}
	p.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))

	// Extract wikilinks from non-code regions.
	p.WikiLinks = extractWikiLinks(raw)

	return p
}

// splitLines splits s into lines, handling \r\n and \n.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	// Normalize \r\n to \n then split.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

// extractWikiLinks extracts unique wikilink targets from markdown text,
// ignoring links inside fenced code blocks and inline code spans.
func extractWikiLinks(s string) []string {
	// Strip fenced code blocks first.
	s = stripFencedCodeBlocks(s)
	// Strip inline code spans.
	s = stripInlineCode(s)

	seen := make(map[string]bool)
	var links []string
	for _, m := range wikilinkRE.FindAllStringSubmatch(s, -1) {
		target := m[1]
		if target == "" {
			continue
		}
		if !seen[target] {
			seen[target] = true
			links = append(links, target)
		}
	}
	return links
}

var fencedBlockRE = regexp.MustCompile("(?s)```.*?```")

// stripFencedCodeBlocks replaces fenced code block contents with whitespace
// so that wikilinks inside them are not extracted.
func stripFencedCodeBlocks(s string) string {
	return fencedBlockRE.ReplaceAllStringFunc(s, func(block string) string {
		// Replace with same-length whitespace to preserve line numbers.
		return strings.Repeat(" ", len(block))
	})
}

var inlineCodeRE = regexp.MustCompile("`[^`]*`")

// stripInlineCode replaces inline code spans with whitespace.
func stripInlineCode(s string) string {
	return inlineCodeRE.ReplaceAllStringFunc(s, func(span string) string {
		return strings.Repeat(" ", len(span))
	})
}
