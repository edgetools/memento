package pages

import (
	"strings"
	"unicode"
)

// illegalRune reports whether r is illegal in a filename on any of
// Linux, macOS, or Windows.
func illegalRune(r rune) bool {
	// Windows-illegal: < > : " / \ | ? *
	// Linux/macOS-illegal: / and null byte
	// We also strip control characters.
	switch r {
	case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
		return true
	}
	return r < 0x20
}

// Normalize returns the canonical form of a page name: lowercase with
// all whitespace runs collapsed to a single space and leading/trailing
// whitespace trimmed.
func Normalize(name string) string {
	// Collapse all whitespace runs (space, tab, etc.) to a single space.
	fields := strings.FieldsFunc(name, unicode.IsSpace)
	return strings.ToLower(strings.Join(fields, " "))
}

// NameToFilename converts a page name to a cross-platform safe filename.
// The result is lowercase, spaces replaced by hyphens, illegal
// characters stripped, consecutive hyphens collapsed, and ".md" appended.
// The stem is truncated to 200 characters before the extension.
func NameToFilename(name string) string {
	// Lowercase first.
	s := strings.ToLower(name)

	// Build the stem character by character.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsSpace(r) {
			b.WriteRune('-')
		} else if illegalRune(r) {
			// skip
		} else {
			b.WriteRune(r)
		}
	}
	stem := b.String()

	// Collapse consecutive hyphens.
	for strings.Contains(stem, "--") {
		stem = strings.ReplaceAll(stem, "--", "-")
	}

	// Trim leading/trailing hyphens and dots.
	stem = strings.Trim(stem, "-.")

	// Truncate to 200 runes.
	runes := []rune(stem)
	if len(runes) > 200 {
		runes = runes[:200]
		stem = string(runes)
		// Re-trim in case truncation left a trailing hyphen/dot.
		stem = strings.Trim(stem, "-.")
	}

	return stem + ".md"
}

// FilenameToName converts a filename back to a human-readable page name
// by stripping the ".md" suffix and replacing hyphens with spaces.
func FilenameToName(filename string) string {
	name := strings.TrimSuffix(filename, ".md")
	return strings.ReplaceAll(name, "-", " ")
}

// NamesMatch reports whether two page names refer to the same page
// (case-insensitive, whitespace-normalized comparison).
func NamesMatch(a, b string) bool {
	return Normalize(a) == Normalize(b)
}
