package index

import (
	"strings"

	"github.com/edgetools/memento/pages"
)

// Graph is a bidirectional wikilink graph.
type Graph struct {
	outbound  map[string]map[string]bool // normalized source → set of normalized targets
	inbound   map[string]map[string]bool // normalized target → set of normalized referrers
	canonical map[string]string          // normalized name → canonical display name
}

// NewGraph creates an empty Graph.
func NewGraph() *Graph {
	return &Graph{
		outbound:  make(map[string]map[string]bool),
		inbound:   make(map[string]map[string]bool),
		canonical: make(map[string]string),
	}
}

func normalizePageName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// Add adds or replaces a page and its outbound wikilinks in the graph.
// If the page was previously indexed, old link relationships are cleaned up first.
func (g *Graph) Add(page pages.Page) {
	key := normalizePageName(page.Name)
	g.canonical[key] = page.Name

	// Remove stale inbound refs from the previous version of this page.
	if old, exists := g.outbound[key]; exists {
		for tNorm := range old {
			if refs, ok := g.inbound[tNorm]; ok {
				delete(refs, key)
			}
		}
	}

	// Build new outbound set, deduplicating targets.
	newOut := make(map[string]bool)
	for _, link := range page.WikiLinks {
		tNorm := normalizePageName(link)
		if tNorm == "" {
			continue
		}
		newOut[tNorm] = true
		// Record the canonical name for this target (first-seen wins).
		if _, exists := g.canonical[tNorm]; !exists {
			g.canonical[tNorm] = link
		}
	}
	g.outbound[key] = newOut

	// Update inbound refs for each target.
	for tNorm := range newOut {
		if g.inbound[tNorm] == nil {
			g.inbound[tNorm] = make(map[string]bool)
		}
		g.inbound[tNorm][key] = true
	}
}

// Remove removes a page and cleans up all its outbound link relationships.
func (g *Graph) Remove(name string) {
	key := normalizePageName(name)
	if out, exists := g.outbound[key]; exists {
		for tNorm := range out {
			if refs, ok := g.inbound[tNorm]; ok {
				delete(refs, key)
			}
		}
		delete(g.outbound, key)
	}
	delete(g.canonical, key)
}

// LinksTo returns the canonical names of pages that the given page links to.
func (g *Graph) LinksTo(name string) []string {
	key := normalizePageName(name)
	out := g.outbound[key]
	if len(out) == 0 {
		return nil
	}
	result := make([]string, 0, len(out))
	for tNorm := range out {
		if canon, ok := g.canonical[tNorm]; ok {
			result = append(result, canon)
		} else {
			result = append(result, tNorm)
		}
	}
	return result
}

// LinkedFrom returns the canonical names of pages that link to the given target.
func (g *Graph) LinkedFrom(name string) []string {
	key := normalizePageName(name)
	refs := g.inbound[key]
	if len(refs) == 0 {
		return nil
	}
	result := make([]string, 0, len(refs))
	for refNorm := range refs {
		if canon, ok := g.canonical[refNorm]; ok {
			result = append(result, canon)
		} else {
			result = append(result, refNorm)
		}
	}
	return result
}
