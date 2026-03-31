package index

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/edgetools/memento/pages"
)

const (
	bm25K1          = 1.5
	bm25B           = 0.75
	weightTitle     = 10.0
	weightLinks     = 3.0
	weightBody      = 1.0
)

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true,
	"but": true, "in": true, "on": true, "at": true, "to": true,
	"for": true, "of": true, "with": true, "by": true, "from": true,
	"is": true, "it": true, "as": true, "be": true, "was": true,
	"are": true, "not": true, "that": true, "this": true, "have": true,
	"had": true, "has": true, "will": true, "would": true, "could": true,
	"should": true, "may": true, "might": true, "do": true, "does": true,
	"did": true, "he": true, "she": true, "we": true, "they": true,
	"you": true, "i": true, "its": true, "also": true, "been": true,
	"were": true, "their": true, "there": true, "when": true, "which": true,
	"some": true, "no": true, "if": true, "so": true, "up": true,
	"out": true, "about": true, "into": true, "than": true, "then": true,
	"over": true, "such": true, "after": true, "before": true, "can": true,
	"all": true, "other": true, "more": true, "very": true, "any": true,
	"what": true, "how": true, "who": true, "my": true, "your": true,
}

// SearchResult is a single result from a BM25 search.
type SearchResult struct {
	Name  string
	Score float64
}

// fieldData holds the per-field term frequencies and token count for one doc.
type fieldData struct {
	tf     map[string]int
	length int
}

// docEntry holds the indexed data for a single page.
type docEntry struct {
	name  string
	title fieldData
	links fieldData
	body  fieldData
}

// BM25 is a weighted-field BM25 inverted index.
type BM25 struct {
	docs          map[string]*docEntry // normalized-name → entry
	docFreq       map[string]int       // term → number of docs containing it
	totalTitleLen int
	totalLinksLen int
	totalBodyLen  int
}

// NewBM25 creates an empty BM25 index.
func NewBM25() *BM25 {
	return &BM25{
		docs:    make(map[string]*docEntry),
		docFreq: make(map[string]int),
	}
}

// Add indexes a page, replacing any existing entry with the same name.
func (b *BM25) Add(page pages.Page) {
	key := strings.ToLower(page.Name)
	// Remove existing entry first so docFreqs are correct.
	if old, exists := b.docs[key]; exists {
		b.removeEntry(old)
	}

	entry := &docEntry{
		name:  page.Name,
		title: tokenizeField(page.Title),
		links: tokenizeLinks(page.WikiLinks),
		body:  tokenizeField(page.Body),
	}

	b.docs[key] = entry
	b.totalTitleLen += entry.title.length
	b.totalLinksLen += entry.links.length
	b.totalBodyLen += entry.body.length

	// Update doc frequencies: a term counts once per doc regardless of field.
	seen := make(map[string]bool)
	for t := range entry.title.tf {
		if !seen[t] {
			seen[t] = true
			b.docFreq[t]++
		}
	}
	for t := range entry.links.tf {
		if !seen[t] {
			seen[t] = true
			b.docFreq[t]++
		}
	}
	for t := range entry.body.tf {
		if !seen[t] {
			seen[t] = true
			b.docFreq[t]++
		}
	}
}

// Remove deletes a page from the index by name.
func (b *BM25) Remove(name string) {
	key := strings.ToLower(name)
	entry, exists := b.docs[key]
	if !exists {
		return
	}
	b.removeEntry(entry)
	delete(b.docs, key)
}

func (b *BM25) removeEntry(entry *docEntry) {
	b.totalTitleLen -= entry.title.length
	b.totalLinksLen -= entry.links.length
	b.totalBodyLen -= entry.body.length

	seen := make(map[string]bool)
	for t := range entry.title.tf {
		if !seen[t] {
			seen[t] = true
			b.docFreq[t]--
			if b.docFreq[t] <= 0 {
				delete(b.docFreq, t)
			}
		}
	}
	for t := range entry.links.tf {
		if !seen[t] {
			seen[t] = true
			b.docFreq[t]--
			if b.docFreq[t] <= 0 {
				delete(b.docFreq, t)
			}
		}
	}
	for t := range entry.body.tf {
		if !seen[t] {
			seen[t] = true
			b.docFreq[t]--
			if b.docFreq[t] <= 0 {
				delete(b.docFreq, t)
			}
		}
	}
}

// Search returns up to limit pages ranked by BM25 score for query.
func (b *BM25) Search(query string, limit int) []SearchResult {
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil
	}
	return b.searchTerms(terms, limit)
}

// searchTerms scores documents for the given pre-stemmed terms and returns ranked results.
// This is the internal implementation used by both Search and the composite Index.
func (b *BM25) searchTerms(terms []string, limit int) []SearchResult {
	N := len(b.docs)
	if N == 0 {
		return nil
	}

	avgTitle := avgLen(b.totalTitleLen, N)
	avgLinks := avgLen(b.totalLinksLen, N)
	avgBody := avgLen(b.totalBodyLen, N)

	scores := make(map[string]float64, N)

	for _, term := range terms {
		df := b.docFreq[term]
		if df == 0 {
			continue
		}
		idf := math.Log((float64(N-df)+0.5)/(float64(df)+0.5) + 1)

		for _, entry := range b.docs {
			score := 0.0
			score += weightTitle * bm25TF(entry.title.tf[term], entry.title.length, avgTitle) * idf
			score += weightLinks * bm25TF(entry.links.tf[term], entry.links.length, avgLinks) * idf
			score += weightBody * bm25TF(entry.body.tf[term], entry.body.length, avgBody) * idf
			if score > 0 {
				scores[entry.name] += score
			}
		}
	}

	results := make([]SearchResult, 0, len(scores))
	for name, score := range scores {
		results = append(results, SearchResult{Name: name, Score: score})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

func bm25TF(tf, docLen int, avgLen float64) float64 {
	if tf == 0 {
		return 0
	}
	tfF := float64(tf)
	norm := bm25K1 * (1 - bm25B + bm25B*float64(docLen)/avgLen)
	return tfF * (bm25K1 + 1) / (tfF + norm)
}

func avgLen(total, n int) float64 {
	if n == 0 {
		return 1
	}
	avg := float64(total) / float64(n)
	if avg == 0 {
		return 1
	}
	return avg
}

// tokenize lowercases, splits on non-letter/digit boundaries, stems, and removes stop words.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var tokens []string
	seen := make(map[string]bool)
	for _, w := range words {
		if stopWords[w] {
			continue
		}
		s := porterStem(w)
		if s == "" {
			continue
		}
		if !seen[s] {
			seen[s] = true
			tokens = append(tokens, s)
		}
	}
	return tokens
}

// tokenizeField tokenizes body/title text, counting term frequencies (with duplicates).
func tokenizeField(text string) fieldData {
	text = strings.ToLower(text)
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	tf := make(map[string]int)
	length := 0
	for _, w := range words {
		if stopWords[w] {
			continue
		}
		s := porterStem(w)
		if s == "" {
			continue
		}
		tf[s]++
		length++
	}
	return fieldData{tf: tf, length: length}
}

// tokenizeLinks tokenizes wikilink targets, indexing individual terms and compound phrases.
func tokenizeLinks(links []string) fieldData {
	tf := make(map[string]int)
	length := 0
	for _, link := range links {
		lower := strings.ToLower(link)
		words := strings.Fields(lower)

		// Index individual terms.
		for _, w := range words {
			if stopWords[w] {
				continue
			}
			s := porterStem(w)
			if s == "" {
				continue
			}
			tf[s]++
			length++
		}

		// Also index the compound phrase (lowercased, joined by space) if multi-word.
		if len(words) > 1 {
			// Stem each word in the compound.
			var stemmed []string
			for _, w := range words {
				if !stopWords[w] {
					s := porterStem(w)
					if s != "" {
						stemmed = append(stemmed, s)
					}
				}
			}
			if len(stemmed) > 1 {
				compound := strings.Join(stemmed, " ")
				tf[compound]++
				length++
			}
		}
	}
	return fieldData{tf: tf, length: length}
}
