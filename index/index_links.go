package index

// LinksTo returns the canonical names of pages that the given page links to.
// It delegates to the underlying graph.
func (ix *Index) LinksTo(name string) []string {
	return ix.graph.LinksTo(name)
}

// LinkedFrom returns the canonical names of pages that link to the given page.
// It delegates to the underlying graph.
func (ix *Index) LinkedFrom(name string) []string {
	return ix.graph.LinkedFrom(name)
}
