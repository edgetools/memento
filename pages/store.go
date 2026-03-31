package pages

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store provides filesystem-backed page storage in a flat directory of
// markdown files. All name lookups are case-insensitive with whitespace
// normalization; the actual heading in each file is the canonical name.
type Store struct {
	dir string
}

// NewStore creates a new Store backed by dir.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// filePath returns the absolute path for the file corresponding to name.
// Because NameToFilename lowercases the name, this is inherently
// case-insensitive.
func (s *Store) filePath(name string) string {
	return filepath.Join(s.dir, NameToFilename(name))
}

// Write creates or fully replaces a page. The file heading is managed
// internally: any existing H1 in content is replaced with "# name", and
// if no H1 is present one is prepended. Returns the parsed Page on success.
func (s *Store) Write(name, content string) (Page, error) {
	fileContent := manageHeading(name, content)

	path := s.filePath(name)
	if err := os.WriteFile(path, []byte(fileContent), 0644); err != nil {
		return Page{}, fmt.Errorf("write page %q: %w", name, err)
	}

	p := Parse(name, []byte(fileContent))
	p.Name = name
	return p, nil
}

// Load reads a page by name (case-insensitive). The returned Page.Name is
// derived from the H1 heading in the file, which is the canonical casing.
func (s *Store) Load(name string) (Page, error) {
	path := s.filePath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Page{}, fmt.Errorf("page not found: %q", name)
		}
		return Page{}, fmt.Errorf("load page %q: %w", name, err)
	}

	// Parse using a placeholder name; we derive the canonical Name from
	// the H1 heading (Title) extracted by the parser.
	p := Parse(FilenameToName(NameToFilename(name)), data)
	p.Name = p.Title
	return p, nil
}

// Delete removes a page by name (case-insensitive).
func (s *Store) Delete(name string) error {
	path := s.filePath(name)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("page not found: %q", name)
		}
		return fmt.Errorf("delete page %q: %w", name, err)
	}
	return nil
}

// Rename renames a page and updates its heading. The source is found
// case-insensitively. Returns an error if the source does not exist or
// the target already exists.
func (s *Store) Rename(oldName, newName string) error {
	oldPath := s.filePath(oldName)
	newPath := s.filePath(newName)

	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return fmt.Errorf("page not found: %q", oldName)
	}

	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("page already exists: %q", newName)
	}

	data, err := os.ReadFile(oldPath)
	if err != nil {
		return fmt.Errorf("rename page %q: %w", oldName, err)
	}

	// Replace the heading with the new name.
	fileContent := manageHeading(newName, string(data))

	if err := os.WriteFile(newPath, []byte(fileContent), 0644); err != nil {
		return fmt.Errorf("rename page %q -> %q: %w", oldName, newName, err)
	}

	if err := os.Remove(oldPath); err != nil {
		os.Remove(newPath) // best-effort rollback
		return fmt.Errorf("rename page %q: remove old file: %w", oldName, err)
	}

	return nil
}

// Scan reads all .md files in the store directory and returns their parsed
// Pages. Files that cannot be read are silently skipped. Returns an empty
// (non-nil) slice when no pages exist.
func (s *Store) Scan() []Page {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return []Page{}
	}

	var result []Page
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name := FilenameToName(entry.Name())
		p := Parse(name, data)
		p.Name = p.Title
		result = append(result, p)
	}
	if result == nil {
		return []Page{}
	}
	return result
}

// Exists reports whether a page with the given name exists (case-insensitive).
func (s *Store) Exists(name string) bool {
	_, err := os.Stat(s.filePath(name))
	return err == nil
}

// manageHeading ensures fileContent starts with "# name\n".
// If content already opens with an H1 line it is replaced; otherwise the
// heading is prepended.
func manageHeading(name, content string) string {
	heading := "# " + name + "\n"

	if strings.HasPrefix(content, "# ") {
		// Strip the existing H1 (first line).
		idx := strings.Index(content, "\n")
		if idx >= 0 {
			content = content[idx+1:]
		} else {
			content = ""
		}
	}

	return heading + content
}
