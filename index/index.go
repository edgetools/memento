package index

import (
	"maps"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/edgetools/memento/embed"
	"github.com/edgetools/memento/pages"
)

const (
	// graphBoostDampened is the score multiplier for pages surfaced via graph boost
	// (not a direct BM25 match, but linked from one). Must be > relevanceRatio so
	// graph-boosted pages survive the relevance threshold.
	graphBoostDampened = 0.6

	// graphBoostMultiplier is applied to pages that are both a direct BM25 match
	// and linked from another direct match.
	graphBoostMultiplier = 1.5

	// relevanceRatio is the fraction of the top score below which results are dropped.
	relevanceRatio = 0.5

	// trigramFuzzyThresh is the minimum Jaccard similarity for a trigram fuzzy match.
	trigramFuzzyThresh = 0.4

	// trigramMinResults is the BM25 result count below which the trigram fallback fires.
	trigramMinResults = 3

	// maxSnippetLen is the maximum snippet length in bytes.
	maxSnippetLen = 300

	// vectorMinScore is the minimum cosine similarity for a vector result to enter the merge.
	vectorMinScore = 0.3
)

// Result is a single result from the composite Index search.
type Result struct {
	Page     string
	Score    float64
	Snippet  string
	Line     int
	IsDirect bool // true if the page matched the query directly (BM25), false if graph-boosted only
}

// Index is the composite search index combining BM25, trigram fuzzy matching,
// a bidirectional wikilink graph for link-boost, and an optional vector index.
type Index struct {
	bm25   *BM25
	tri    *Trigram
	graph  *Graph
	vector *VectorIndex // nil when no embedding model is available
	pages  map[string]pages.Page // normalized name → stored page (for snippets)
}

// NewIndex creates an empty composite Index. When model is non-nil a VectorIndex
// is created and wired in; when nil the index behaves as BM25 + trigram + graph only.
func NewIndex(model *embed.Model) *Index {
	ix := &Index{
		bm25:  NewBM25(),
		tri:   NewTrigram(),
		graph: NewGraph(),
		pages: make(map[string]pages.Page),
	}
	if model != nil {
		ix.vector = NewVectorIndex(model)
	}
	return ix
}

// Add indexes a page, replacing any existing entry with the same name.
func (ix *Index) Add(page pages.Page) {
	key := strings.ToLower(page.Name)
	ix.bm25.Add(page)
	ix.graph.Add(page)
	ix.pages[key] = page

	// Feed all stemmed terms from this page into the trigram index for fuzzy matching.
	for term := range collectPageTerms(page) {
		ix.tri.Add(term)
	}

	if ix.vector != nil {
		ix.vector.Add(page) //nolint:errcheck // embedding errors are non-fatal
	}
}

// Remove removes a page from all sub-indexes.
func (ix *Index) Remove(name string) {
	key := strings.ToLower(name)
	ix.bm25.Remove(name)
	ix.graph.Remove(name)
	delete(ix.pages, key)
	if ix.vector != nil {
		ix.vector.Remove(name)
	}
}

// Search executes the full search pipeline and returns up to limit results.
//
// Pipeline (with vector model):
//   BM25 + vector cosine search → normalize & merge → graph boost → relevance threshold.
//
// Pipeline (nil model, backward-compatible):
//   BM25 → trigram fallback if <3 results → graph boost → relevance threshold.
func (ix *Index) Search(query string, limit int) []Result {
	if query == "" || len(ix.pages) == 0 {
		return nil
	}

	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil
	}

	// Layer 1: BM25 keyword search.
	bm25Raw := ix.bm25.searchTerms(queryTerms, 0)

	// Layer 2: vector search (model present) or trigram fallback (nil model).
	var vecResults []VectorResult
	if ix.vector != nil {
		vecResults = ix.runVectorSearch(query)
	} else if len(bm25Raw) < trigramMinResults {
		expanded := ix.expandTerms(queryTerms)
		if len(expanded) > len(queryTerms) {
			bm25Raw = ix.bm25.searchTerms(expanded, 0)
		}
	}

	// Layer 3: build direct-match score map, merging BM25 and vector when available.
	bm25Scores := make(map[string]float64, len(bm25Raw))
	for _, r := range bm25Raw {
		bm25Scores[r.Name] = r.Score
	}

	// vecLineHints maps canonical page name → best-chunk start line from vector results.
	vecLineHints := make(map[string]int, len(vecResults))
	for _, vr := range vecResults {
		vecLineHints[vr.Page] = vr.Line
	}

	var directScores map[string]float64
	if ix.vector != nil {
		directScores = ix.mergeScores(bm25Scores, vecResults)
	} else {
		directScores = bm25Scores
	}

	// Layer 4: graph boost — add linked pages and boost doubly-connected pages.
	finalScores, referrers := ix.graphBoost(directScores)

	// Layer 5: relevance threshold — drop results below 50% of the top score.
	topScore := 0.0
	for _, s := range finalScores {
		if s > topScore {
			topScore = s
		}
	}
	if topScore > 0 {
		thresh := topScore * relevanceRatio
		for name, s := range finalScores {
			if s < thresh {
				delete(finalScores, name)
			}
		}
	}

	// Sort by score descending.
	type entry struct {
		name  string
		score float64
	}
	ranked := make([]entry, 0, len(finalScores))
	for name, score := range finalScores {
		ranked = append(ranked, entry{name, score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}

	// Layer 6: build results with snippets.
	results := make([]Result, 0, len(ranked))
	for _, e := range ranked {
		key := strings.ToLower(e.name)
		page, ok := ix.pages[key]
		if !ok {
			continue
		}

		bm25Hit := bm25Scores[e.name] > 0
		_, vecHit := vecLineHints[e.name]
		isDirect := bm25Hit || (ix.vector != nil && vecHit)
		referrer := referrers[e.name]

		var snippet string
		var line int
		if !bm25Hit && vecHit {
			// Vector-only match: anchor snippet on the best-matching chunk's line.
			snippet, line = vectorSnippet(page, vecLineHints[e.name])
		} else {
			snippet, line = ix.buildSnippet(page, queryTerms, isDirect, referrer)
		}

		results = append(results, Result{
			Page:     e.name,
			Score:    e.score,
			Snippet:  snippet,
			Line:     line,
			IsDirect: isDirect,
		})
	}
	return results
}

// runVectorSearch runs the vector index search and filters out low-similarity results.
func (ix *Index) runVectorSearch(query string) []VectorResult {
	raw := ix.vector.Search(query, 20)
	out := make([]VectorResult, 0, len(raw))
	for _, vr := range raw {
		if vr.Score >= vectorMinScore {
			out = append(out, vr)
		}
	}
	return out
}

// mergeScores builds the direct-match score map from BM25 and vector results.
//
// Vector is the semantic gate: only pages found by vector (score ≥ 0.3,
// already filtered by runVectorSearch) enter the merged set. BM25 provides
// a score boost for pages that appear in both pipelines. Pages found only by
// BM25 (no vector support) are excluded — a coincidental keyword match in a
// semantically unrelated page should not survive when the model is present.
//
// When vector returns no results, BM25 scores pass through normalized so that
// keyword-only searches still work.
func (ix *Index) mergeScores(bm25Scores map[string]float64, vecResults []VectorResult) map[string]float64 {
	// No vector results: normalize BM25 and pass through.
	if len(vecResults) == 0 {
		bm25Top := 0.0
		for _, s := range bm25Scores {
			bm25Top = max(bm25Top, s)
		}
		merged := make(map[string]float64, len(bm25Scores))
		for name, s := range bm25Scores {
			if bm25Top > 0 {
				merged[name] = s / bm25Top
			} else {
				merged[name] = s
			}
		}
		return merged
	}

	// Vector has results: find top scores for normalization.
	bm25Top := 0.0
	for _, s := range bm25Scores {
		bm25Top = max(bm25Top, s)
	}
	vecTop := 0.0
	for _, vr := range vecResults {
		vecTop = max(vecTop, vr.Score)
	}

	// Build merged set from vector results only, boosting with BM25 where available.
	merged := make(map[string]float64, len(vecResults))
	for _, vr := range vecResults {
		normVec := vr.Score
		if vecTop > 0 {
			normVec = vr.Score / vecTop
		}
		normBM25 := 0.0
		if s, ok := bm25Scores[vr.Page]; ok && bm25Top > 0 {
			normBM25 = s / bm25Top
		}
		merged[vr.Page] = max(normVec, normBM25)
	}
	return merged
}

// vectorSnippet extracts a snippet anchored at lineHint (1-indexed page line)
// from the page body. Falls back to the first paragraph if the anchor is empty.
func vectorSnippet(page pages.Page, lineHint int) (string, int) {
	lines := strings.Split(page.Body, "\n")
	if len(lines) == 0 {
		return "", lineHint
	}
	// lineHint is a 1-indexed page line; body[0] is page line 2.
	bodyIdx := max(0, min(lineHint-2, len(lines)-1))
	start := max(0, bodyIdx-1)
	end := min(len(lines), bodyIdx+2)
	snippet := truncateSnippet(strings.TrimSpace(strings.Join(lines[start:end], " ")))
	if snippet != "" {
		return snippet, lineHint
	}
	// Fallback: first paragraph.
	return firstParagraphSnippet(page)
}

// expandTerms expands query terms using trigram fuzzy matching.
func (ix *Index) expandTerms(terms []string) []string {
	expanded := make([]string, len(terms))
	copy(expanded, terms)
	seen := make(map[string]bool, len(terms))
	for _, t := range terms {
		seen[t] = true
	}
	for _, t := range terms {
		for _, match := range ix.tri.FuzzyMatch(t, trigramFuzzyThresh) {
			if !seen[match] {
				seen[match] = true
				expanded = append(expanded, match)
			}
		}
	}
	return expanded
}

// graphBoost adds graph-connected pages to the score map and boosts doubly-connected pages.
// Returns the final score map and a referrer map (graph-boosted page → referring direct-match page).
func (ix *Index) graphBoost(direct map[string]float64) (map[string]float64, map[string]string) {
	final := make(map[string]float64, len(direct)*2)
	referrers := make(map[string]string)

	// Seed with direct scores.
	maps.Copy(final, direct)

	// For each direct match, traverse one hop in both directions.
	for srcName, srcScore := range direct {
		// Outbound: pages this direct match links to.
		for _, target := range ix.graph.LinksTo(srcName) {
			if dScore, isDirect := direct[target]; isDirect {
				// Target is also a direct match → multiply its score.
				boosted := dScore * graphBoostMultiplier
				if boosted > final[target] {
					final[target] = boosted
				}
			} else {
				// Target is graph-only → add with dampened score.
				dampened := srcScore * graphBoostDampened
				if dampened > final[target] {
					final[target] = dampened
					referrers[target] = srcName
				}
			}
		}
		// Inbound: pages that link to this direct match.
		for _, referrer := range ix.graph.LinkedFrom(srcName) {
			if dScore, isDirect := direct[referrer]; isDirect {
				boosted := dScore * graphBoostMultiplier
				if boosted > final[referrer] {
					final[referrer] = boosted
				}
			} else {
				dampened := srcScore * graphBoostDampened
				if dampened > final[referrer] {
					final[referrer] = dampened
					referrers[referrer] = srcName
				}
			}
		}
	}

	return final, referrers
}

// buildSnippet generates a contextual snippet and line number for a result.
func (ix *Index) buildSnippet(page pages.Page, queryTerms []string, isDirect bool, referrer string) (string, int) {
	// Graph-boosted result: snippet comes from the referring page showing [[PageName]].
	if !isDirect && referrer != "" {
		refKey := strings.ToLower(referrer)
		if refPage, ok := ix.pages[refKey]; ok {
			return referrerSnippet(refPage, page.Name)
		}
	}

	// Title match: use first paragraph.
	titleTokens := tokenizeField(page.Title)
	for _, qt := range queryTerms {
		if titleTokens.tf[qt] > 0 {
			return firstParagraphSnippet(page)
		}
	}

	// Direct body match: find the line with highest query term density.
	return densitySnippet(page, queryTerms)
}

// referrerSnippet extracts ~250 chars centered on [[targetName]] in the referring page.
func referrerSnippet(refPage pages.Page, targetName string) (string, int) {
	body := refPage.Body
	pattern := regexp.MustCompile(`(?i)\[\[` + regexp.QuoteMeta(targetName) + `\]\]`)
	loc := pattern.FindStringIndex(body)
	if loc == nil {
		// Fallback: first line of referring body.
		first := strings.SplitN(body, "\n", 2)[0]
		return truncateSnippet(first), 2
	}

	// Expand ~125 chars on each side of the match.
	snipStart := max(0, loc[0]-125)
	snipEnd := min(len(body), loc[1]+125)

	snippet := body[snipStart:snipEnd]
	if snipStart > 0 {
		snippet = "..." + snippet
	}
	if snipEnd < len(body) {
		snippet = snippet + "..."
	}

	// Line number: heading is line 1, body starts at line 2.
	linesBefore := strings.Count(body[:loc[0]], "\n")
	return snippet, linesBefore + 2
}

// firstParagraphSnippet returns the first paragraph of the page body and line 2.
func firstParagraphSnippet(page pages.Page) (string, int) {
	body := strings.TrimSpace(page.Body)
	firstPara, _, _ := strings.Cut(body, "\n\n")
	firstPara = strings.TrimSpace(firstPara)
	return truncateSnippet(firstPara), 2
}

// densitySnippet finds the line with the most query term matches and returns surrounding context.
func densitySnippet(page pages.Page, queryTerms []string) (string, int) {
	lines := strings.Split(page.Body, "\n")
	if len(lines) == 0 {
		return "", 2
	}

	bestIdx := 0
	bestCount := -1
	for i, line := range lines {
		if count := countTermMatches(line, queryTerms); count > bestCount {
			bestCount = count
			bestIdx = i
		}
	}

	// Extract the best line with one line of context on each side.
	start := max(0, bestIdx-1)
	end := min(len(lines), bestIdx+2)
	snippet := strings.Join(lines[start:end], " ")
	return truncateSnippet(snippet), bestIdx + 2 // +2: heading is line 1, body starts at line 2
}

// countTermMatches counts how many unique query stems appear in text.
func countTermMatches(text string, queryTerms []string) int {
	text = strings.ToLower(text)
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	termSet := make(map[string]bool, len(queryTerms))
	for _, t := range queryTerms {
		termSet[t] = true
	}
	count := 0
	seen := make(map[string]bool)
	for _, w := range words {
		s := porterStem(w)
		if termSet[s] && !seen[s] {
			seen[s] = true
			count++
		}
	}
	return count
}

// truncateSnippet truncates s to at most maxSnippetLen bytes.
func truncateSnippet(s string) string {
	if len(s) <= maxSnippetLen {
		return s
	}
	return s[:maxSnippetLen]
}

// collectPageTerms returns all unique stemmed terms across all fields of a page.
func collectPageTerms(page pages.Page) map[string]bool {
	terms := make(map[string]bool)
	for t := range tokenizeField(page.Title).tf {
		terms[t] = true
	}
	for t := range tokenizeLinks(page.WikiLinks).tf {
		terms[t] = true
	}
	for t := range tokenizeField(page.Body).tf {
		terms[t] = true
	}
	return terms
}
