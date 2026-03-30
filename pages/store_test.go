package pages_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edgetools/memento/pages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTestPage writes a raw markdown file directly to dir, bypassing the store.
// Used to set up pre-existing file state without going through heading management.
func writeTestPage(t *testing.T, dir, name, content string) {
	t.Helper()
	filename := pages.NameToFilename(name)
	err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644)
	require.NoError(t, err)
}

// readTestPage reads the raw file content for a page from dir, bypassing the store.
// Used to assert on-disk state independently of the store's Load method.
func readTestPage(t *testing.T, dir, name string) string {
	t.Helper()
	filename := pages.NameToFilename(name)
	data, err := os.ReadFile(filepath.Join(dir, filename))
	require.NoError(t, err)
	return string(data)
}

// countMDFiles returns the number of .md files directly in dir.
func countMDFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			n++
		}
	}
	return n
}

// pageNames extracts the Name field from a slice of pages.
func pageNames(pp []pages.Page) []string {
	names := make([]string, len(pp))
	for i, p := range pp {
		names[i] = p.Name
	}
	return names
}

// ---- Write ----------------------------------------------------------------

func TestWrite(t *testing.T) {
	t.Parallel()

	t.Run("creates_new_page", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Crowd Control", "Crowd control refers to abilities that limit enemy movement.")
		require.NoError(t, err)

		raw := readTestPage(t, dir, "Crowd Control")
		assert.Contains(t, raw, "Crowd control refers to abilities that limit enemy movement.")
	})

	t.Run("adds_heading", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Crowd Control", "Some body text.")
		require.NoError(t, err)

		raw := readTestPage(t, dir, "Crowd Control")
		assert.True(t,
			strings.HasPrefix(raw, "# Crowd Control\n"),
			"file should start with '# Crowd Control\\n', got: %q", raw,
		)
	})

	t.Run("replaces_agent_heading", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Crowd Control", "# Wrong Heading\n\nBody text.")
		require.NoError(t, err)

		raw := readTestPage(t, dir, "Crowd Control")
		assert.Contains(t, raw, "# Crowd Control")
		assert.NotContains(t, raw, "# Wrong Heading")
		assert.Contains(t, raw, "Body text.")
	})

	t.Run("overwrites_existing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Enchanter", "First content.")
		require.NoError(t, err)
		_, err = s.Write("Enchanter", "Second content.")
		require.NoError(t, err)

		raw := readTestPage(t, dir, "Enchanter")
		assert.Contains(t, raw, "Second content.")
		assert.NotContains(t, raw, "First content.")
	})

	t.Run("case_insensitive_overwrite", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Foo", "First version.")
		require.NoError(t, err)
		_, err = s.Write("foo", "Second version.")
		require.NoError(t, err)

		assert.Equal(t, 1, countMDFiles(t, dir), "should be exactly one .md file after case-insensitive overwrite")
		raw := readTestPage(t, dir, "foo")
		assert.Contains(t, raw, "Second version.")
		assert.NotContains(t, raw, "First version.")
	})

	t.Run("returns_parsed_page", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		p, err := s.Write("Enchanter", "The enchanter uses [[Mez]] and [[Haste]].")
		require.NoError(t, err)

		assert.Equal(t, "Enchanter", p.Name)
		assert.ElementsMatch(t, []string{"Mez", "Haste"}, p.WikiLinks)
	})

	t.Run("empty_content", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		p, err := s.Write("Empty Page", "")
		require.NoError(t, err)

		assert.Equal(t, "Empty Page", p.Name)
		raw := readTestPage(t, dir, "Empty Page")
		assert.Contains(t, raw, "# Empty Page")
	})
}

// ---- Load -----------------------------------------------------------------

func TestLoad(t *testing.T) {
	t.Parallel()

	t.Run("reads_existing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Crowd Control", "Body content here.")
		require.NoError(t, err)

		p, err := s.Load("Crowd Control")
		require.NoError(t, err)
		assert.Contains(t, p.Body, "Body content here.")
	})

	t.Run("case_insensitive", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Foo Bar", "Some content.")
		require.NoError(t, err)

		p, err := s.Load("foo bar")
		require.NoError(t, err)
		// The canonical name from the heading should be returned.
		assert.Equal(t, "Foo Bar", p.Name)
	})

	t.Run("whitespace_normalized", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Foo Bar", "Some content.")
		require.NoError(t, err)

		// Extra internal space should still resolve to the same page.
		p, err := s.Load("Foo  Bar")
		require.NoError(t, err)
		assert.Equal(t, "Foo Bar", p.Name)
	})

	t.Run("not_found_errors", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Load("Nonexistent Page")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Nonexistent Page")
	})

	t.Run("returns_parsed_fields", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Enchanter", "The enchanter uses [[Mez]] and [[Crowd Control]].")
		require.NoError(t, err)

		p, err := s.Load("Enchanter")
		require.NoError(t, err)

		assert.Equal(t, "Enchanter", p.Title)
		assert.ElementsMatch(t, []string{"Mez", "Crowd Control"}, p.WikiLinks)
		assert.Contains(t, p.Body, "The enchanter uses")
		assert.Greater(t, p.Lines, 0)
	})
}

// ---- Delete ---------------------------------------------------------------

func TestDelete(t *testing.T) {
	t.Parallel()

	t.Run("removes_file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Crowd Control", "Some content.")
		require.NoError(t, err)
		require.True(t, s.Exists("Crowd Control"))

		err = s.Delete("Crowd Control")
		require.NoError(t, err)

		assert.False(t, s.Exists("Crowd Control"))
		assert.Equal(t, 0, countMDFiles(t, dir))
	})

	t.Run("case_insensitive", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Foo", "Some content.")
		require.NoError(t, err)

		err = s.Delete("foo")
		require.NoError(t, err)

		assert.False(t, s.Exists("Foo"))
		assert.Equal(t, 0, countMDFiles(t, dir))
	})

	t.Run("not_found_errors", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		err := s.Delete("Nonexistent Page")
		require.Error(t, err)
	})
}

// ---- Rename ---------------------------------------------------------------

func TestRename(t *testing.T) {
	t.Parallel()

	t.Run("changes_filename", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Old Name", "Some content.")
		require.NoError(t, err)

		err = s.Rename("Old Name", "New Name")
		require.NoError(t, err)

		assert.False(t, s.Exists("Old Name"), "old page should no longer exist")
		assert.True(t, s.Exists("New Name"), "new page should exist")
		// Verify the old filename is gone and the new one is present.
		_, statErr := os.Stat(filepath.Join(dir, pages.NameToFilename("Old Name")))
		assert.True(t, os.IsNotExist(statErr), "old file should be removed from disk")
		_, statErr = os.Stat(filepath.Join(dir, pages.NameToFilename("New Name")))
		assert.NoError(t, statErr, "new file should be on disk")
	})

	t.Run("preserves_content", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Old Name", "Body content that should survive the rename.")
		require.NoError(t, err)

		err = s.Rename("Old Name", "New Name")
		require.NoError(t, err)

		p, err := s.Load("New Name")
		require.NoError(t, err)
		assert.Contains(t, p.Body, "Body content that should survive the rename.")
	})

	t.Run("updates_heading", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Old Name", "Some body.")
		require.NoError(t, err)

		err = s.Rename("Old Name", "New Name")
		require.NoError(t, err)

		raw := readTestPage(t, dir, "New Name")
		assert.True(t,
			strings.HasPrefix(raw, "# New Name\n"),
			"renamed file should open with '# New Name\\n', got: %q", raw,
		)
		assert.NotContains(t, raw, "# Old Name")
	})

	t.Run("case_insensitive_source", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Foo Bar", "Content here.")
		require.NoError(t, err)

		// Source supplied in all-lowercase — should still find the page.
		err = s.Rename("foo bar", "Baz Qux")
		require.NoError(t, err)

		assert.False(t, s.Exists("Foo Bar"))
		assert.True(t, s.Exists("Baz Qux"))
	})

	t.Run("target_exists_errors", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Page A", "Content A.")
		require.NoError(t, err)
		_, err = s.Write("Page B", "Content B.")
		require.NoError(t, err)

		err = s.Rename("Page A", "Page B")
		require.Error(t, err)

		// Neither page should have been disturbed.
		assert.True(t, s.Exists("Page A"))
		assert.True(t, s.Exists("Page B"))
	})

	t.Run("source_not_found_errors", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		err := s.Rename("Nonexistent", "New Name")
		require.Error(t, err)
	})
}

// ---- Scan -----------------------------------------------------------------

func TestScan(t *testing.T) {
	t.Parallel()

	t.Run("returns_all", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Page One", "Content one.")
		require.NoError(t, err)
		_, err = s.Write("Page Two", "Content two.")
		require.NoError(t, err)
		_, err = s.Write("Page Three", "Content three.")
		require.NoError(t, err)

		result := s.Scan()
		require.Len(t, result, 3)
		// Names should reflect canonical casing from the headings.
		assert.ElementsMatch(t, []string{"Page One", "Page Two", "Page Three"}, pageNames(result))
	})

	t.Run("empty_dir", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		result := s.Scan()
		assert.Empty(t, result)
	})

	t.Run("ignores_non_md", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Real Page", "Content.")
		require.NoError(t, err)

		// Non-.md file written directly to the content dir.
		err = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("some text"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(dir, "README"), []byte("readme"), 0644)
		require.NoError(t, err)

		result := s.Scan()
		require.Len(t, result, 1)
		assert.Equal(t, "Real Page", result[0].Name)
	})

	t.Run("returns_parsed_pages", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Enchanter", "Uses [[Mez]] and [[Haste]].")
		require.NoError(t, err)

		result := s.Scan()
		require.Len(t, result, 1)
		p := result[0]
		assert.Equal(t, "Enchanter", p.Name)
		assert.Equal(t, "Enchanter", p.Title)
		assert.ElementsMatch(t, []string{"Mez", "Haste"}, p.WikiLinks)
	})
}

// ---- Exists ---------------------------------------------------------------

func TestExists(t *testing.T) {
	t.Parallel()

	t.Run("true_for_existing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Enchanter", "Content.")
		require.NoError(t, err)

		assert.True(t, s.Exists("Enchanter"))
	})

	t.Run("false_for_missing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		assert.False(t, s.Exists("Nonexistent"))
	})

	t.Run("case_insensitive", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := pages.NewStore(dir)

		_, err := s.Write("Foo", "Content.")
		require.NoError(t, err)

		assert.True(t, s.Exists("Foo"))
		assert.True(t, s.Exists("foo"))
		assert.True(t, s.Exists("FOO"))
	})
}
