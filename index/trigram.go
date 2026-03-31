package index

// Trigrams returns the set of unique 3-character sliding windows for term.
// For terms shorter than 3 characters, returns the term itself as a single element.
func Trigrams(term string) []string {
	runes := []rune(term)
	if len(runes) == 0 {
		return nil
	}
	if len(runes) < 3 {
		return []string{term}
	}
	seen := make(map[string]bool)
	var result []string
	for i := 0; i <= len(runes)-3; i++ {
		t := string(runes[i : i+3])
		if !seen[t] {
			seen[t] = true
			result = append(result, t)
		}
	}
	return result
}

// trigramSet returns a term's trigrams as a set.
func trigramSet(term string) map[string]bool {
	tgrams := Trigrams(term)
	set := make(map[string]bool, len(tgrams))
	for _, t := range tgrams {
		set[t] = true
	}
	return set
}

// Similarity computes the Jaccard similarity of the trigram sets of two strings.
func Similarity(a, b string) float64 {
	return jaccardSets(trigramSet(a), trigramSet(b))
}

func jaccardSets(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	intersection := 0
	for t := range a {
		if b[t] {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

// Trigram is an in-memory fuzzy-match index based on trigram Jaccard similarity.
type Trigram struct {
	terms map[string]map[string]bool // term → trigram set
}

// NewTrigram creates an empty trigram index.
func NewTrigram() *Trigram {
	return &Trigram{terms: make(map[string]map[string]bool)}
}

// Add adds a term to the trigram index.
func (ti *Trigram) Add(term string) {
	ti.terms[term] = trigramSet(term)
}

// FuzzyMatch returns all indexed terms whose Jaccard similarity with query
// is at or above threshold.
func (ti *Trigram) FuzzyMatch(query string, threshold float64) []string {
	if query == "" {
		return nil
	}
	querySet := trigramSet(query)
	var matches []string
	for term, termSet := range ti.terms {
		if jaccardSets(querySet, termSet) >= threshold {
			matches = append(matches, term)
		}
	}
	return matches
}
