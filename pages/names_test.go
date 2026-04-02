package pages_test

import (
	"strings"
	"testing"

	"github.com/edgetools/memento/pages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	t.Run("preserves_casing", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "Crowd Control.md", pages.NameToFilename("Crowd Control"))
	})

	t.Run("preserves_spaces", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "Aggro From Healing.md", pages.NameToFilename("Aggro From Healing"))
	})

	t.Run("simple_single_word", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "Enchanter.md", pages.NameToFilename("Enchanter"))
	})

	t.Run("appends_md_extension", func(t *testing.T) {
		t.Parallel()
		result := pages.NameToFilename("Any Page")
		assert.True(t, strings.HasSuffix(result, ".md"), "result should end with .md, got: %q", result)
	})

	t.Run("long_name_truncated", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("a", 300)
		result := pages.NameToFilename(long)
		// stem capped at 200 runes + ".md"
		assert.LessOrEqual(t, len([]rune(result)), 203)
		assert.True(t, strings.HasSuffix(result, ".md"))
	})

	t.Run("long_name_truncated_unicode", func(t *testing.T) {
		t.Parallel()
		// Multi-byte runes: 300 runes to exercise the truncation path.
		long := strings.Repeat("Ü", 300)
		result := pages.NameToFilename(long)
		assert.LessOrEqual(t, len([]rune(result)), 203)
		assert.True(t, strings.HasSuffix(result, ".md"))
	})

	t.Run("roundtrip", func(t *testing.T) {
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
				// Direct filename→name round-trip should recover the original name exactly.
				require.Equal(t, name, recovered)
			})
		}
	})
}

func TestNameToFilenameValidation(t *testing.T) {
	t.Parallel()

	// Characters forbidden on Windows, macOS, Linux, iOS, or Android —
	// the same set Obsidian rejects.
	forbiddenChars := []struct {
		name string
		char string
	}{
		{"asterisk", "*"},
		{"double_quote", `"`},
		{"open_bracket", "["},
		{"close_bracket", "]"},
		{"hash", "#"},
		{"caret", "^"},
		{"pipe", "|"},
		{"less_than", "<"},
		{"greater_than", ">"},
		{"colon", ":"},
		{"question_mark", "?"},
		{"forward_slash", "/"},
		{"backslash", `\`},
	}
	for _, fc := range forbiddenChars {
		t.Run("rejects_"+fc.name, func(t *testing.T) {
			t.Parallel()
			name := "Page" + fc.char + "Name"
			_, err := pages.ValidateName(name)
			require.Error(t, err, "expected error for page name containing %q", fc.char)
			assert.Contains(t, err.Error(), fc.char)
		})
	}

	t.Run("rejects_leading_dot", func(t *testing.T) {
		t.Parallel()
		_, err := pages.ValidateName(".hidden")
		require.Error(t, err)
	})

	t.Run("accepts_valid_name", func(t *testing.T) {
		t.Parallel()
		filename, err := pages.ValidateName("Crowd Control")
		require.NoError(t, err)
		assert.Equal(t, "Crowd Control.md", filename)
	})

	t.Run("accepts_unicode", func(t *testing.T) {
		t.Parallel()
		filename, err := pages.ValidateName("Über Strategy")
		require.NoError(t, err)
		assert.Equal(t, "Über Strategy.md", filename)
	})

	t.Run("accepts_hyphens_and_underscores", func(t *testing.T) {
		t.Parallel()
		filename, err := pages.ValidateName("my-page_name")
		require.NoError(t, err)
		assert.Equal(t, "my-page_name.md", filename)
	})

	t.Run("empty_name_errors", func(t *testing.T) {
		t.Parallel()
		_, err := pages.ValidateName("")
		require.Error(t, err)
	})
}

func TestFilenameToName(t *testing.T) {
	t.Parallel()

	t.Run("strips_md_extension", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "Crowd Control", pages.FilenameToName("Crowd Control.md"))
	})

	t.Run("preserves_casing", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "My Page Title", pages.FilenameToName("My Page Title.md"))
	})

	t.Run("preserves_spaces", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "Page With Spaces", pages.FilenameToName("Page With Spaces.md"))
	})
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
