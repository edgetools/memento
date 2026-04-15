package index_test

import (
	"strings"
	"testing"

	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// above50words contains exactly 51 space-separated tokens (no internal newlines).
// Used to build chunk content that is clearly above the 50-token minimum size threshold.
const above50words = "one two three four five six seven eight nine ten " +
	"eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty " +
	"twentyone twentytwo twentythree twentyfour twentyfive twentysix twentyseven twentyeight twentynine thirty " +
	"thirtyone thirtytwo thirtythree thirtyfour thirtyfive thirtysix thirtyseven thirtyeight thirtynine forty " +
	"fortyone fortytwo fortythree fortyfour fortyfive fortysix fortyseven fortyeight fortynine fifty " +
	"fiftyone"

// below50words contains 11 space-separated tokens (no internal newlines).
// Used to build chunk content that is clearly below the 50-token minimum size threshold.
const below50words = "This is a short section with only a few words total."

// containsAnyChunk reports whether any chunk's Text contains substr.
func containsAnyChunk(chunks []index.Chunk, substr string) bool {
	for _, c := range chunks {
		if strings.Contains(c.Text, substr) {
			return true
		}
	}
	return false
}

func TestChunkPage(t *testing.T) {
	t.Parallel()

	// ── Single-chunk edge cases ──────────────────────────────────────────────

	t.Run("title_only_produces_single_chunk", func(t *testing.T) {
		t.Parallel()
		// Page with only a title and no body whatsoever.
		p := pages.Parse("empty-page", []byte("# Empty Page"))
		chunks := index.ChunkPage(p)
		require.Len(t, chunks, 1, "title-only page should produce exactly one chunk")
		assert.Contains(t, chunks[0].Text, "# Empty Page",
			"chunk text should contain the page title")
		assert.Equal(t, 1, chunks[0].StartLine,
			"single chunk should start at line 1")
		assert.Equal(t, 1, chunks[0].EndLine,
			"single chunk should end at line 1")
	})

	t.Run("empty_body_produces_single_chunk", func(t *testing.T) {
		t.Parallel()
		// Page with a title followed by only whitespace/blank lines.
		p := pages.Parse("my-page", []byte("# My Page\n\n"))
		chunks := index.ChunkPage(p)
		require.Len(t, chunks, 1, "page with empty body should produce exactly one chunk")
		assert.True(t, strings.HasPrefix(chunks[0].Text, "# My Page"),
			"chunk text should start with the page title")
	})

	t.Run("small_page_no_headings_single_chunk", func(t *testing.T) {
		t.Parallel()
		// Body has fewer than 50 tokens and no section headings.
		// The entire page should collapse into one chunk.
		raw := "# Small Page\n\n" + below50words
		p := pages.Parse("small-page", []byte(raw))
		chunks := index.ChunkPage(p)
		require.Len(t, chunks, 1,
			"page with body < 50 tokens should be a single chunk")
		assert.True(t, strings.HasPrefix(chunks[0].Text, "# Small Page\n"),
			"chunk text should be prefixed with the page title")
		assert.Contains(t, chunks[0].Text, below50words,
			"chunk text should contain the body content")
	})

	t.Run("no_headings_no_paragraphs_single_chunk", func(t *testing.T) {
		t.Parallel()
		// Large body with no headings and no double-newline paragraph breaks.
		// No split points → single chunk.
		raw := "# My Page\n\n" + above50words
		p := pages.Parse("my-page", []byte(raw))
		chunks := index.ChunkPage(p)
		require.Len(t, chunks, 1,
			"large body with no split points should be a single chunk")
	})

	// ── Title prefix ─────────────────────────────────────────────────────────

	t.Run("all_chunks_prefixed_with_title", func(t *testing.T) {
		t.Parallel()
		// Two large sections force multiple chunks; every chunk must carry the title prefix.
		raw := "# My Page\n\n" +
			"## Section One\n" + above50words + "\n\n" +
			"## Section Two\n" + above50words
		p := pages.Parse("my-page", []byte(raw))
		chunks := index.ChunkPage(p)
		require.True(t, len(chunks) >= 2, "two large sections should produce at least 2 chunks")
		for i, chunk := range chunks {
			assert.True(t, strings.HasPrefix(chunk.Text, "# My Page\n"),
				"chunk %d should be prefixed with '# My Page\\n'", i)
		}
	})

	// ── Section heading splitting ─────────────────────────────────────────────

	t.Run("section_heading_creates_split", func(t *testing.T) {
		t.Parallel()
		// Two large sections separated by ## headings.
		raw := "# Doc\n\n" +
			"## First\n" + above50words + "\n\n" +
			"## Second\n" + above50words
		p := pages.Parse("doc", []byte(raw))
		chunks := index.ChunkPage(p)
		require.True(t, len(chunks) >= 2,
			"each ## section should produce a separate chunk")
		assert.True(t, containsAnyChunk(chunks, "## First"),
			"a chunk should contain '## First'")
		assert.True(t, containsAnyChunk(chunks, "## Second"),
			"a chunk should contain '## Second'")
	})

	t.Run("intro_chunk_before_first_heading", func(t *testing.T) {
		t.Parallel()
		// Large body content appears before the first ## heading (the "intro" chunk).
		// Since intro is above the minimum size, it must not be merged into the section.
		raw := "# My Page\n\n" +
			above50words + "\n\n" +
			"## Section\n" + above50words
		p := pages.Parse("my-page", []byte(raw))
		chunks := index.ChunkPage(p)
		require.True(t, len(chunks) >= 2,
			"large intro + section should produce at least 2 chunks")
		// The first chunk should contain intro content.
		assert.Contains(t, chunks[0].Text, "fortyone",
			"first chunk should contain intro content from above50words")
		// The intro chunk must end before the ## Section heading.
		assert.NotContains(t, chunks[0].Text, "## Section",
			"intro chunk should not include the section heading")
	})

	t.Run("h2_and_h3_both_are_split_points", func(t *testing.T) {
		t.Parallel()
		// Both ## and ### should trigger chunk splits (not just ##).
		raw := "# Doc\n\n" +
			"## H2 Section\n" + above50words + "\n\n" +
			"### H3 Subsection\n" + above50words
		p := pages.Parse("doc", []byte(raw))
		chunks := index.ChunkPage(p)
		require.True(t, len(chunks) >= 2,
			"both ## and ### should be split points")
		assert.True(t, containsAnyChunk(chunks, "## H2 Section"),
			"chunk with H2 heading expected")
		assert.True(t, containsAnyChunk(chunks, "### H3 Subsection"),
			"chunk with H3 heading expected")
	})

	t.Run("single_h1_not_a_split_point", func(t *testing.T) {
		t.Parallel()
		// A line starting with a single # (level-1 heading) must NOT be a section split.
		// Only ## and higher (^#{2,} ) trigger splits.
		raw := "# Title\n\n" + above50words + "\n\n# Another H1\n" + above50words
		p := pages.Parse("title", []byte(raw))
		chunks := index.ChunkPage(p)
		// All chunks should carry the page title prefix, not another H1 as an opener.
		for i, chunk := range chunks {
			assert.True(t, strings.HasPrefix(chunk.Text, "# Title\n"),
				"chunk %d should be prefixed with the page title, not treated as a split at the second H1", i)
		}
	})

	// ── Paragraph fallback (no headings) ─────────────────────────────────────

	t.Run("paragraph_fallback_when_no_headings", func(t *testing.T) {
		t.Parallel()
		// No ## headings: should split on \n\n paragraph breaks.
		// Two large paragraphs should produce at least 2 chunks.
		raw := "# My Page\n\n" + above50words + "\n\n" + above50words
		p := pages.Parse("my-page", []byte(raw))
		chunks := index.ChunkPage(p)
		require.True(t, len(chunks) >= 2,
			"no headings + two large paragraphs should produce 2+ chunks via fallback split")
	})

	t.Run("paragraph_fallback_not_used_when_headings_exist", func(t *testing.T) {
		t.Parallel()
		// When ## headings are present, paragraph breaks inside a section must NOT split.
		// A section containing two consecutive large paragraphs stays as one section chunk.
		twoParaSection := above50words + "\n\n" + above50words
		raw := "# Doc\n\n## Only Section\n" + twoParaSection
		p := pages.Parse("doc", []byte(raw))
		chunks := index.ChunkPage(p)
		// Only the ## heading is a split point; the internal \n\n must not create extra splits.
		// All content belongs to the one (possibly intro-merged) section chunk.
		require.True(t, containsAnyChunk(chunks, "## Only Section"),
			"should have a chunk containing the section heading")
		// Find the section chunk and confirm it holds all content from both paragraphs.
		for _, c := range chunks {
			if strings.Contains(c.Text, "## Only Section") {
				// The section chunk should contain content from BOTH paragraphs (above50words appears twice).
				assert.True(t, strings.Count(c.Text, "fiftyone") >= 2,
					"section chunk should contain all paragraph content within the section, not be paragraph-split")
			}
		}
	})

	// ── Minimum size merging ──────────────────────────────────────────────────

	t.Run("small_chunk_merges_forward_into_next", func(t *testing.T) {
		t.Parallel()
		// A small section (< 50 tokens) is followed by a large one.
		// The small section must merge into the large one.
		// No intro body before the first ## → intro is empty and also merges.
		// Expected result: a single merged chunk containing both section headings.
		raw := "# Doc\n\n" +
			"## Small Section\n" + below50words + "\n\n" +
			"## Large Section\n" + above50words
		p := pages.Parse("doc", []byte(raw))
		chunks := index.ChunkPage(p)
		require.Len(t, chunks, 1,
			"small section (< 50 tokens) should merge with the following large section")
		assert.Contains(t, chunks[0].Text, "## Small Section",
			"merged chunk should contain small section heading")
		assert.Contains(t, chunks[0].Text, "## Large Section",
			"merged chunk should contain large section heading")
	})

	t.Run("small_last_chunk_merges_backward_into_previous", func(t *testing.T) {
		t.Parallel()
		// A large section followed by a small final section.
		// The small final section must merge backward into the preceding chunk.
		raw := "# Doc\n\n" +
			"## Large Section\n" + above50words + "\n\n" +
			"## Small Section\n" + below50words
		p := pages.Parse("doc", []byte(raw))
		chunks := index.ChunkPage(p)
		require.Len(t, chunks, 1,
			"small last section should merge backward with the preceding chunk")
		assert.Contains(t, chunks[0].Text, "## Large Section",
			"merged chunk should contain large section heading")
		assert.Contains(t, chunks[0].Text, "## Small Section",
			"merged chunk should contain small last section heading")
	})

	t.Run("all_small_sections_produce_single_chunk", func(t *testing.T) {
		t.Parallel()
		// Every section is below the minimum; cascading merges should yield exactly one chunk.
		raw := "# Doc\n\n" +
			"## Section One\n" + below50words + "\n\n" +
			"## Section Two\n" + below50words + "\n\n" +
			"## Section Three\n" + below50words
		p := pages.Parse("doc", []byte(raw))
		chunks := index.ChunkPage(p)
		require.Len(t, chunks, 1,
			"all sections below minimum should cascade-merge into a single chunk")
	})

	// ── Code block: headings inside fenced blocks are not split points ────────

	t.Run("heading_inside_fenced_code_block_not_a_split", func(t *testing.T) {
		t.Parallel()
		// A ## inside a ``` fenced block must be treated as plain text, not a section split.
		// The only real heading is ## Real Section. After the empty intro merges into it,
		// result is a single chunk that contains both the real heading and the fake one.
		raw := "# Doc\n\n" +
			"## Real Section\n" + above50words + "\n\n" +
			"```\n## Fake Heading\n```\n\n" +
			above50words
		p := pages.Parse("doc", []byte(raw))
		chunks := index.ChunkPage(p)
		// With correct behavior: only ## Real Section splits; ## Fake Heading does not.
		// The empty intro merges with Real Section → 1 chunk total.
		require.Len(t, chunks, 1,
			"## inside a fenced code block must not be treated as a section split point")
		assert.Contains(t, chunks[0].Text, "## Fake Heading",
			"code block content should appear as text within the chunk")
		assert.Contains(t, chunks[0].Text, "## Real Section",
			"real section heading should be present")
	})

	// ── Line number tracking ──────────────────────────────────────────────────

	t.Run("first_chunk_starts_at_line_1", func(t *testing.T) {
		t.Parallel()
		// Regardless of page structure, the first chunk must always start at line 1
		// (the # Title line is line 1 of the raw content).
		raw := "# My Page\n\n" + above50words
		p := pages.Parse("my-page", []byte(raw))
		chunks := index.ChunkPage(p)
		require.NotEmpty(t, chunks)
		assert.Equal(t, 1, chunks[0].StartLine,
			"first chunk must start at line 1 — the title line")
	})

	t.Run("section_chunk_startline_matches_heading_line", func(t *testing.T) {
		t.Parallel()
		// The StartLine of a section chunk must equal the 1-indexed line number of its ## heading.
		raw := "# My Page\n\n" + // lines 1-2
			above50words + "\n\n" + // line 3 (above50words is one line) + line 4
			"## Section\n" + // the heading line
			above50words // section body line
		p := pages.Parse("my-page", []byte(raw))
		chunks := index.ChunkPage(p)
		require.True(t, len(chunks) >= 2,
			"large intro + large section should produce at least 2 chunks")

		// Dynamically locate the line number of "## Section" in the raw content.
		rawLines := strings.Split(raw, "\n")
		sectionLine := 0
		for i, line := range rawLines {
			if line == "## Section" {
				sectionLine = i + 1 // 1-indexed
				break
			}
		}
		require.Greater(t, sectionLine, 0, "'## Section' must be present in the raw content")

		// Find the chunk that owns the section.
		var sectionChunk *index.Chunk
		for i := range chunks {
			if strings.Contains(chunks[i].Text, "## Section") {
				sectionChunk = &chunks[i]
				break
			}
		}
		require.NotNil(t, sectionChunk, "should find a chunk containing '## Section'")
		assert.Equal(t, sectionLine, sectionChunk.StartLine,
			"section chunk StartLine should equal the line number of its ## heading")
	})

	t.Run("last_chunk_ends_at_last_line_of_page", func(t *testing.T) {
		t.Parallel()
		// The final chunk's EndLine must equal the total line count of the raw content.
		raw := "# My Page\n\n" +
			"## Section One\n" + above50words + "\n\n" +
			"## Section Two\n" + above50words
		p := pages.Parse("my-page", []byte(raw))
		chunks := index.ChunkPage(p)
		require.NotEmpty(t, chunks)

		totalLines := strings.Count(raw, "\n") + 1
		lastChunk := chunks[len(chunks)-1]
		assert.Equal(t, totalLines, lastChunk.EndLine,
			"last chunk EndLine must equal the page's total line count")
	})

	t.Run("section_chunks_have_ascending_line_ranges", func(t *testing.T) {
		t.Parallel()
		// Each chunk's StartLine must be greater than the previous chunk's StartLine,
		// and each chunk's EndLine must be >= its StartLine.
		raw := "# My Page\n\n" +
			above50words + "\n\n" +
			"## Section One\n" + above50words + "\n\n" +
			"## Section Two\n" + above50words
		p := pages.Parse("my-page", []byte(raw))
		chunks := index.ChunkPage(p)
		require.True(t, len(chunks) >= 2, "should have multiple chunks")

		for i, chunk := range chunks {
			assert.GreaterOrEqual(t, chunk.EndLine, chunk.StartLine,
				"chunk %d EndLine must be >= StartLine", i)
			if i > 0 {
				assert.Greater(t, chunk.StartLine, chunks[i-1].StartLine,
					"chunk %d StartLine must be greater than chunk %d StartLine", i, i-1)
			}
		}
	})

	t.Run("chunk_line_ranges_cover_full_page", func(t *testing.T) {
		t.Parallel()
		// The first chunk starts at line 1 and the last ends at the page's final line.
		// Together the chunks span the entire document.
		raw := "# My Page\n\n" +
			above50words + "\n\n" +
			"## Section\n" + above50words
		p := pages.Parse("my-page", []byte(raw))
		chunks := index.ChunkPage(p)
		require.NotEmpty(t, chunks)

		totalLines := strings.Count(raw, "\n") + 1
		assert.Equal(t, 1, chunks[0].StartLine,
			"first chunk must start at line 1")
		assert.Equal(t, totalLines, chunks[len(chunks)-1].EndLine,
			"last chunk must end at the final line of the page")
	})
}
