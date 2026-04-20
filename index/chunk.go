package index

import (
	"regexp"
	"strings"

	"github.com/edgetools/memento/pages"
)

// Chunk represents a semantically-meaningful portion of a page with
// line-range tracking for search result anchoring.
type Chunk struct {
	Text      string // chunk content, prefixed with the page's # Title line
	StartLine int    // 1-indexed start line in the original page content
	EndLine   int    // 1-indexed end line in the original page content (inclusive)
}

// sectionHeadingRE matches lines that are section headings (## or deeper).
// Single # (H1) is the page title and is NOT a section split point.
var sectionHeadingRE = regexp.MustCompile(`^#{2,} `)

const minChunkTokens = 50

// ChunkPage splits a page into semantically-meaningful chunks using section
// headings as the primary split strategy, falling back to paragraph breaks
// when no headings are present. Every chunk is prefixed with the page's
// "# Title" line. Chunks whose body content is below minChunkTokens tokens
// are merged with an adjacent chunk.
func ChunkPage(page pages.Page) []Chunk {
	titleLine := "# " + page.Title

	// Empty body: the whole page is one chunk (just the title).
	if page.Body == "" {
		return []Chunk{{
			Text:      titleLine,
			StartLine: 1,
			EndLine:   page.Lines,
		}}
	}

	bodyLines := strings.Split(page.Body, "\n")

	// Compute the 1-indexed line number of bodyLines[0] in the original content.
	// original lines = 1 (title) + gap (blank lines after title) + len(bodyLines)
	// => bodyStartLine = page.Lines - len(bodyLines) + 1
	bodyStartLine := page.Lines - len(bodyLines) + 1

	// Choose split strategy based on whether section headings exist.
	var raw []rawChunk
	if idxs := findHeadingIndices(bodyLines); len(idxs) > 0 {
		raw = splitOnHeadings(bodyLines, bodyStartLine, idxs)
	} else {
		raw = splitOnParagraphs(bodyLines, bodyStartLine)
	}

	// The first chunk always starts at line 1 (the title line is its anchor).
	// The last chunk always ends at the final line of the page.
	raw[0].startLine = 1
	raw[len(raw)-1].endLine = page.Lines

	// Merge any chunks that are too small.
	raw = mergeSmallChunks(raw, minChunkTokens)

	// Build final Chunk objects with the title prefix.
	result := make([]Chunk, len(raw))
	for i, rc := range raw {
		result[i] = Chunk{
			Text:      titleLine + "\n" + rc.bodyText,
			StartLine: rc.startLine,
			EndLine:   rc.endLine,
		}
	}
	return result
}

// rawChunk is an intermediate representation used during chunking before the
// title prefix is applied.
type rawChunk struct {
	bodyText  string // chunk body (no title prefix)
	startLine int    // 1-indexed start line in original page
	endLine   int    // 1-indexed end line in original page (inclusive)
}

// findHeadingIndices returns the body-line indices of section headings (##+),
// skipping any lines inside fenced code blocks (``` delimiters).
func findHeadingIndices(bodyLines []string) []int {
	var idxs []int
	inFence := false
	for i, line := range bodyLines {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
		}
		if !inFence && sectionHeadingRE.MatchString(line) {
			idxs = append(idxs, i)
		}
	}
	return idxs
}

// splitOnHeadings splits bodyLines into raw chunks at each section heading.
// Content before the first heading becomes an "intro" chunk.
// headingIdxs must be non-empty.
func splitOnHeadings(bodyLines []string, bodyStartLine int, headingIdxs []int) []rawChunk {
	var chunks []rawChunk

	// Intro chunk: lines before the first heading (may be empty, skip if so).
	if headingIdxs[0] > 0 {
		intro := bodyLines[:headingIdxs[0]]
		chunks = append(chunks, rawChunk{
			bodyText:  strings.Join(intro, "\n"),
			startLine: bodyStartLine,
			endLine:   bodyStartLine + headingIdxs[0] - 1,
		})
	}

	// One chunk per section heading.
	for i, hIdx := range headingIdxs {
		endIdx := len(bodyLines)
		if i+1 < len(headingIdxs) {
			endIdx = headingIdxs[i+1]
		}
		chunks = append(chunks, rawChunk{
			bodyText:  strings.Join(bodyLines[hIdx:endIdx], "\n"),
			startLine: bodyStartLine + hIdx,
			endLine:   bodyStartLine + endIdx - 1,
		})
	}

	return chunks
}

// splitOnParagraphs splits bodyLines on runs of blank lines (≥1 empty line).
// This is the fallback when no section headings are present.
func splitOnParagraphs(bodyLines []string, bodyStartLine int) []rawChunk {
	var chunks []rawChunk
	var current []string
	currentStart := bodyStartLine
	inBlankRun := false

	for i, line := range bodyLines {
		if line == "" {
			if !inBlankRun && len(current) > 0 {
				// Close the current paragraph.
				chunks = append(chunks, rawChunk{
					bodyText:  strings.Join(current, "\n"),
					startLine: currentStart,
					endLine:   bodyStartLine + i - 1,
				})
				current = nil
			}
			inBlankRun = true
		} else {
			if inBlankRun {
				// First non-blank line after a blank run: start a new paragraph.
				currentStart = bodyStartLine + i
				inBlankRun = false
			}
			current = append(current, line)
		}
	}

	// Flush the final paragraph.
	if len(current) > 0 {
		chunks = append(chunks, rawChunk{
			bodyText:  strings.Join(current, "\n"),
			startLine: currentStart,
			endLine:   bodyStartLine + len(bodyLines) - 1,
		})
	}

	// If there were no blank lines, the whole body is one chunk.
	if len(chunks) == 0 {
		chunks = append(chunks, rawChunk{
			bodyText:  strings.Join(bodyLines, "\n"),
			startLine: bodyStartLine,
			endLine:   bodyStartLine + len(bodyLines) - 1,
		})
	}

	return chunks
}

// mergeSmallChunks iteratively merges any chunk whose body token count is
// below minTokens with a neighbor: forward (into the next) if possible,
// backward (into the previous) if it is the last chunk.
func mergeSmallChunks(chunks []rawChunk, minTokens int) []rawChunk {
	for len(chunks) > 1 {
		found := false
		for i := range chunks {
			if tokenCount(chunks[i].bodyText) < minTokens {
				found = true
				var combined rawChunk
				if i+1 < len(chunks) {
					combined = joinChunks(chunks[i], chunks[i+1])
					chunks = replaceTwo(chunks, i, combined)
				} else {
					combined = joinChunks(chunks[i-1], chunks[i])
					chunks = replaceTwo(chunks, i-1, combined)
				}
				break
			}
		}
		if !found {
			break
		}
	}
	return chunks
}

// joinChunks concatenates two adjacent raw chunks into one.
func joinChunks(a, b rawChunk) rawChunk {
	return rawChunk{
		bodyText:  a.bodyText + "\n" + b.bodyText,
		startLine: a.startLine,
		endLine:   b.endLine,
	}
}

// replaceTwo replaces chunks[at] and chunks[at+1] with replacement.
func replaceTwo(chunks []rawChunk, at int, replacement rawChunk) []rawChunk {
	out := make([]rawChunk, 0, len(chunks)-1)
	out = append(out, chunks[:at]...)
	out = append(out, replacement)
	out = append(out, chunks[at+2:]...)
	return out
}

// tokenCount returns the number of whitespace-delimited tokens in text.
func tokenCount(text string) int {
	return len(strings.Fields(text))
}
