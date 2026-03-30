package pages_test

import (
	"strings"
	"testing"

	"github.com/edgetools/memento/pages"
	"github.com/stretchr/testify/assert"
)

func TestNormalize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"lowercase", "Crowd Control", "crowd control"},
		{"mixed_case", "cRoWd CoNtRoL", "crowd control"},
		{"extra_whitespace", "Crowd  Control", "crowd control"},
		{"leading_trailing", "  Crowd Control  ", "crowd control"},
		{"tabs", "Crowd\tControl", "crowd control"},
		{"single_word", "Enchanter", "enchanter"},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, pages.Normalize(tc.input))
		})
	}
}

func TestNameToFilename(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "Crowd Control", "crowd-control.md"},
		{"special_chars", "Aggro From Healing", "aggro-from-healing.md"},
		{"strips_illegal_chars", "What: A <Test>?", "what-a-test.md"},
		{"unicode_preserved", "Über Strategy", "über-strategy.md"},
		{"collapses_hyphens", "A -- B", "a-b.md"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, pages.NameToFilename(tc.input))
		})
	}
}

func TestNameToFilename_LongNameTruncated(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 300)
	result := pages.NameToFilename(long)
	assert.LessOrEqual(t, len(result), 204)
	assert.True(t, strings.HasSuffix(result, ".md"))
}

func TestNameToFilename_Roundtrip(t *testing.T) {
	t.Parallel()
	names := []string{
		"Crowd Control",
		"Enchanter",
		"Aggro From Healing",
		"Simple Page",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			filename := pages.NameToFilename(name)
			recovered := pages.FilenameToName(filename)
			// Recovered name should normalize to the same as the original
			assert.Equal(t, pages.Normalize(name), pages.Normalize(recovered))
		})
	}
}

func TestFilenameToName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "crowd control", pages.FilenameToName("crowd-control.md"))
}

func TestNamesMatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"case_insensitive", "Crowd Control", "crowd control", true},
		{"whitespace_norm", "Crowd  Control", "Crowd Control", true},
		{"different", "Crowd Control", "Enchanter", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, pages.NamesMatch(tc.a, tc.b))
		})
	}
}
