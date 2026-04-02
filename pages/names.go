package pages

import (
	"fmt"
	"strings"
	"unicode"
)

// forbiddenRune reports whether r is forbidden in a page name.
// This matches the set that Obsidian rejects: characters illegal on
// Windows, macOS, Linux, iOS, or Android.
func forbiddenRune(r rune) bool {
	switch r {
	case '*', '"', '[', ']', '#', '^', '|', '<', '>', ':', '?', '/', '\\':
		return true
	}
	return false
}

// ValidateName checks that name is acceptable as a page name and returns
// the corresponding filename. An error is returned if the name is empty,
// starts with '.', or contains any forbidden character.
func ValidateName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("page name must not be empty")
	}
	if strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("page name must not start with '.'")
	}
	for _, r := range name {
		if forbiddenRune(r) {
			return "", fmt.Errorf("page name contains forbidden character %q", string(r))
		}
	}
	return NameToFilename(name), nil
}

// Normalize returns the canonical form of a page name: lowercase with
// all whitespace runs collapsed to a single space and leading/trailing
// whitespace trimmed.
func Normalize(name string) string {
	// Collapse all whitespace runs (space, tab, etc.) to a single space.
	fields := strings.FieldsFunc(name, unicode.IsSpace)
	return strings.ToLower(strings.Join(fields, " "))
}

// NameToFilename converts a page name to a filename by appending ".md".
// The stem is truncated to 200 runes before the extension.
// Casing and spaces are preserved as-is.
func NameToFilename(name string) string {
	// Truncate to 200 runes.
	runes := []rune(name)
	if len(runes) > 200 {
		runes = runes[:200]
		name = string(runes)
	}
	return name + ".md"
}

// FilenameToName converts a filename back to the page name by stripping
// the ".md" suffix. Casing and spaces are preserved.
func FilenameToName(filename string) string {
	return strings.TrimSuffix(filename, ".md")
}

// NamesMatch reports whether two page names refer to the same page
// (case-insensitive, whitespace-normalized comparison).
func NamesMatch(a, b string) bool {
	return Normalize(a) == Normalize(b)
}
