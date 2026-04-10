package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// lastUpdatedForFile resolves the last-modified timestamp for a page file.
//
// Priority order:
//  1. Git commit time — the author/commit time of the most recent commit that
//     touched this file. Preferred because commit times survive git clone/pull.
//  2. Filesystem mtime — used when the content directory is not a git repo, or
//     when the file has never been committed (e.g. newly created, not yet staged).
//  3. Empty string — returned when neither source is available.
//
// The returned string is an ISO 8601 / RFC 3339 timestamp in UTC ending in "Z".
func lastUpdatedForFile(filePath string) string {
	dir := filepath.Dir(filePath)
	filename := filepath.Base(filePath)

	// Try git commit time. %cI gives ISO 8601 with timezone offset.
	cmd := exec.Command("git", "-C", dir, "log", "-1", "--format=%cI", "--", filename)
	out, err := cmd.Output()
	if err == nil {
		ts := strings.TrimSpace(string(out))
		if ts != "" {
			t, parseErr := time.Parse(time.RFC3339, ts)
			if parseErr == nil {
				return t.UTC().Format(time.RFC3339)
			}
		}
	}

	// Fallback: filesystem mtime.
	info, statErr := os.Stat(filePath)
	if statErr == nil {
		return info.ModTime().UTC().Format(time.RFC3339)
	}

	return ""
}
