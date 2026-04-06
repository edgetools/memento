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

// filePath returns the absolute path for the exact filename derived from name.
func (s *Store) filePath(name string) string {
	return filepath.Join(s.dir, NameToFilename(name))
}

// FilePath returns the absolute filesystem path for the page with the given
// name. The path is derived deterministically from the name; the file does not
// need to exist.
func (s *Store) FilePath(name string) string {
	return s.filePath(name)
}

// resolvePath finds the on-disk path for a page by name using
// case-insensitive, whitespace-normalized matching. It first checks
// the exact path; if that does not exist it scans the directory for a
// file whose stem normalizes to the same value as name.
// Returns the resolved path and true on success, or ("", false) if not found.
func (s *Store) resolvePath(name string) (string, bool) {
	exact := s.filePath(name)
	if _, err := os.Stat(exact); err == nil {
		return exact, true
	}

	norm := Normalize(name)
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if Normalize(FilenameToName(entry.Name())) == norm {
			return filepath.Join(s.dir, entry.Name()), true
		}
	}
	return "", false
}

// Write creates or fully replaces a page. The file heading is managed
// internally: any existing H1 in content is replaced with "# name", and
// if no H1 is present one is prepended. Returns the parsed Page on success.
//
// If a file with the same normalized name already exists at a different path
// (e.g. different casing), the old file is removed so only one file remains.
func (s *Store) Write(name, content string) (Page, error) {
	if _, err := ValidateName(name); err != nil {
		return Page{}, fmt.Errorf("write page %q: %w", name, err)
	}

	fileContent := manageHeading(name, content)
	newPath := s.filePath(name)

	// If an existing file matches by normalized name but lives at a different
	// path (different casing), remove it before writing the new file.
	if existingPath, ok := s.resolvePath(name); ok && existingPath != newPath {
		if err := os.Remove(existingPath); err != nil {
			return Page{}, fmt.Errorf("write page %q: remove old file: %w", name, err)
		}
	}

	if err := os.WriteFile(newPath, []byte(fileContent), 0644); err != nil {
		return Page{}, fmt.Errorf("write page %q: %w", name, err)
	}

	p := Parse(name, []byte(fileContent))
	p.Name = name
	return p, nil
}

// Load reads a page by name (case-insensitive, whitespace-normalized).
// The returned Page.Name is derived from the H1 heading in the file,
// which is the canonical casing.
func (s *Store) Load(name string) (Page, error) {
	path, ok := s.resolvePath(name)
	if !ok {
		return Page{}, fmt.Errorf("page not found: %q", name)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Page{}, fmt.Errorf("load page %q: %w", name, err)
	}

	// Parse using the filename-derived name; we derive the canonical Name from
	// the H1 heading (Title) extracted by the parser.
	p := Parse(FilenameToName(filepath.Base(path)), data)
	p.Name = p.Title
	return p, nil
}

// Delete removes a page by name (case-insensitive, whitespace-normalized).
func (s *Store) Delete(name string) error {
	path, ok := s.resolvePath(name)
	if !ok {
		return fmt.Errorf("page not found: %q", name)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete page %q: %w", name, err)
	}
	return nil
}

// Rename renames a page and updates its heading. The source is found
// case-insensitively. Returns an error if the source does not exist or
// the target already exists.
func (s *Store) Rename(oldName, newName string) error {
	if _, err := ValidateName(newName); err != nil {
		return fmt.Errorf("rename page %q -> %q: %w", oldName, newName, err)
	}

	oldPath, ok := s.resolvePath(oldName)
	if !ok {
		return fmt.Errorf("page not found: %q", oldName)
	}
	newPath := s.filePath(newName)

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

// Exists reports whether a page with the given name exists
// (case-insensitive, whitespace-normalized).
func (s *Store) Exists(name string) bool {
	_, ok := s.resolvePath(name)
	return ok
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
