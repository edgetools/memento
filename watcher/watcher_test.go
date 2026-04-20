package watcher_test

// Tests for CR6: Filesystem Watcher.
//
// Verifies that the Watcher detects external changes to .md files in the
// content directory and updates the in-memory index accordingly, without
// requiring a restart.
//
// All tests write real files to a t.TempDir() and wait for the debounce
// window (150 ms) to settle before asserting index state.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/edgetools/memento/watcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// debounceSettle is the wait period used after triggering filesystem events.
// It is longer than the 150 ms debounce window to allow the watcher goroutine
// to fully process events before assertions run.
const debounceSettle = 400 * time.Millisecond

// ---------------------------------------------------------------------------
// Test helpers

// newTestEnv creates a temporary directory, a pages.Store, and an index.Index
// backed by that directory. It does NOT start the watcher — callers do that
// explicitly so they can pre-populate the index before watching.
func newTestEnv(t *testing.T) (dir string, store *pages.Store, idx *index.Index) {
	t.Helper()
	dir = t.TempDir()
	store = pages.NewStore(dir)
	idx = index.NewIndex(nil, "") // nil model = BM25 + trigram only, no vector
	return
}

// populateIndex writes several background pages to dir and adds them to idx.
// Having multiple documents in the BM25 corpus ensures IDF scores are
// non-trivial, making search results more deterministic.
func populateIndex(t *testing.T, store *pages.Store, idx *index.Index) {
	t.Helper()
	for i, name := range []string{"Background One", "Background Two", "Background Three"} {
		page, err := store.Write(name, fmt.Sprintf("Background page %d content.", i+1))
		require.NoError(t, err)
		idx.Add(page)
	}
}

// writeMdRaw writes raw bytes to a .md filename derived from name, bypassing
// the Store so the watcher — not the tool — is responsible for indexing it.
func writeMdRaw(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, pages.NameToFilename(name))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// deleteMdRaw removes the .md file for name from dir.
func deleteMdRaw(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, pages.NameToFilename(name))
	require.NoError(t, os.Remove(path))
}

// indexContainsPage returns true when idx.Search returns a result whose Page
// field case-insensitively matches name.
func indexContainsPage(idx *index.Index, name string) bool {
	results := idx.Search(name, 20)
	for _, r := range results {
		if pages.NamesMatch(r.Page, name) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Construction

func TestNewWatcher_ReturnsWatcherWithNoError(t *testing.T) {
	dir, store, idx := newTestEnv(t)

	w, err := watcher.NewWatcher(dir, store, idx)

	require.NoError(t, err)
	require.NotNil(t, w)
	_ = w.Close()
}

// ---------------------------------------------------------------------------
// Start / Close lifecycle

func TestStart_FirstCallSucceeds(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	defer w.Close()

	err = w.Start()
	assert.NoError(t, err)
}

func TestStart_ReturnErrorIfAlreadyStarted(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	defer w.Close()

	require.NoError(t, w.Start())

	err = w.Start()
	assert.Error(t, err, "second Start() call should return an error")
}

func TestClose_BeforeStartIsSafe(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)

	// Close without ever calling Start should not panic and should return no error.
	assert.NoError(t, w.Close())
}

func TestClose_IdempotentAfterStart(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())

	require.NoError(t, w.Close())

	// Second Close must not panic and should return no error.
	assert.NoError(t, w.Close())
}

// ---------------------------------------------------------------------------
// Create events

func TestWatcher_CreateEvent_AddsPageToIndex(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	populateIndex(t, store, idx)

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	defer w.Close()

	// Write a new .md file externally (bypassing the Store so only the watcher
	// is responsible for indexing it).
	writeMdRaw(t, dir, "new arrival", "# New Arrival\nContent about the new arrival page.")

	time.Sleep(debounceSettle)

	assert.True(t, indexContainsPage(idx, "New Arrival"),
		"page created externally should be in the index after debounce")
}

func TestWatcher_CreateEvent_IgnoresNonMdFiles(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	populateIndex(t, store, idx)

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	defer w.Close()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("# Txt File\nsome text"), 0o644))

	time.Sleep(debounceSettle)

	assert.False(t, indexContainsPage(idx, "Txt File"),
		"non-.md file should not be indexed")
}

func TestWatcher_RenameEvent_IgnoresNonMdFiles(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	populateIndex(t, store, idx)

	// Write a non-.md file before starting the watcher so no Create event fires.
	oldPath := filepath.Join(dir, "notes.txt")
	newPath := filepath.Join(dir, "notes-renamed.txt")
	require.NoError(t, os.WriteFile(oldPath, []byte("some plain text"), 0o644))

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	defer w.Close()

	require.NoError(t, os.Rename(oldPath, newPath))

	time.Sleep(debounceSettle)

	// Neither the old nor the new path should appear in the index.
	assert.False(t, indexContainsPage(idx, "notes"),
		"renaming a non-.md file should not affect the index")
	assert.False(t, indexContainsPage(idx, "notes-renamed"),
		"renaming a non-.md file should not affect the index")
}

func TestWatcher_CreateEvent_IgnoresMementoVectorsFile(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	populateIndex(t, store, idx)

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	defer w.Close()

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".memento-vectors"), []byte("binary cache data"), 0o644))

	time.Sleep(debounceSettle)

	// There is no page named ".memento-vectors", so just verify no new pages
	// appeared in the index (sanity check via a direct search).
	results := idx.Search("memento-vectors", 20)
	for _, r := range results {
		assert.NotEqual(t, ".memento-vectors", r.Page,
			".memento-vectors cache file should not be indexed")
	}
}

func TestWatcher_CreateEvent_IgnoresDirectories(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	populateIndex(t, store, idx)

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	defer w.Close()

	// Create a subdirectory inside the watched directory.
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o755))

	time.Sleep(debounceSettle)

	assert.False(t, indexContainsPage(idx, "subdir"),
		"directory creation should not trigger indexing")
}

// ---------------------------------------------------------------------------
// Modify events

func TestWatcher_ModifyEvent_UpdatesPageInIndex(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	populateIndex(t, store, idx)

	// Pre-populate the page with old content before the watcher starts.
	page, err := store.Write("live page", "Old content before external edit.")
	require.NoError(t, err)
	idx.Add(page)

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	defer w.Close()

	// Overwrite the file externally — simulating a peer Claude session writing
	// through its own memento instance.
	writeMdRaw(t, dir, "live page", "# Live Page\nUpdated content after external edit.")

	time.Sleep(debounceSettle)

	assert.True(t, indexContainsPage(idx, "Live Page"),
		"page should still appear in the index after an external modify")
}

func TestWatcher_ModifyEvent_ReplacesStaleEntry(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	populateIndex(t, store, idx)

	// Pre-populate a page with old content that contains a unique term.
	page, err := store.Write("stale title", "# Stale Title\nOriginal content here.")
	require.NoError(t, err)
	idx.Add(page)

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	defer w.Close()

	// External write: keep same filename but replace the content body with a
	// unique term ("xyzfreshtoken") that cannot appear anywhere else in the index.
	writeMdRaw(t, dir, "stale title", "# Stale Title\nReplaced content with xyzfreshtoken material.")

	time.Sleep(debounceSettle)

	// The unique term from the updated content must be discoverable, proving
	// that the stale index entry was replaced rather than merely retained.
	results := idx.Search("xyzfreshtoken", 20)
	assert.NotEmpty(t, results,
		"unique term from the updated content should be findable after modify re-indexes the page")
}

// ---------------------------------------------------------------------------
// Delete events

func TestWatcher_DeleteEvent_RemovesPageFromIndex(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	populateIndex(t, store, idx)

	// Pre-populate a page before the watcher starts.
	page, err := store.Write("to be deleted", "Content of the page that will be deleted.")
	require.NoError(t, err)
	idx.Add(page)

	// Sanity-check: page is indexed before deletion.
	require.True(t, indexContainsPage(idx, "To Be Deleted"), "pre-condition: page must be indexed before test")

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	defer w.Close()

	deleteMdRaw(t, dir, "to be deleted")

	time.Sleep(debounceSettle)

	assert.False(t, indexContainsPage(idx, "To Be Deleted"),
		"deleted page should be removed from the index")
}

// ---------------------------------------------------------------------------
// Rename events
//
// When a file is renamed, fsnotify fires a Rename event for the old path and a
// Create event for the new path.  The watcher must:
//   - Call idx.Remove for the old filename-derived name (Rename event), and
//   - Call store.Load + idx.Add for the new filename (Create event).

func TestWatcher_RenameEvent_OldNameRemovedFromIndex(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	populateIndex(t, store, idx)

	// Pre-populate the source page.
	page, err := store.Write("rename source", "Content of the source page before rename.")
	require.NoError(t, err)
	idx.Add(page)
	require.True(t, indexContainsPage(idx, "Rename Source"), "pre-condition: source must be indexed")

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	defer w.Close()

	oldPath := filepath.Join(dir, pages.NameToFilename("rename source"))
	newPath := filepath.Join(dir, pages.NameToFilename("rename destination"))
	require.NoError(t, os.Rename(oldPath, newPath))

	time.Sleep(debounceSettle)

	// The filename-derived key "rename-source" must be removed. The page may
	// re-appear under its H1 heading name (which store.Load derives from the
	// file's H1), so we check that the exact old normalized name is gone.
	assert.False(t, indexContainsPage(idx, "rename source"),
		"old filename-derived name should be removed from index after rename")
}

func TestWatcher_RenameEvent_NewFileIsIndexed(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	populateIndex(t, store, idx)

	// Write the source file before starting the watcher so no Create event fires
	// for it; the watcher starts with a clean slate and only sees the Rename.
	writeMdRaw(t, dir, "pre-rename file", "# Pre-Rename File\nContent that travels with the file.")

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	defer w.Close()

	oldPath := filepath.Join(dir, pages.NameToFilename("pre-rename file"))
	newPath := filepath.Join(dir, pages.NameToFilename("post-rename file"))
	require.NoError(t, os.Rename(oldPath, newPath))

	time.Sleep(debounceSettle)

	// After the Create event fires on the new path, the page content should be
	// discoverable (even if the heading-derived name differs from the filename).
	results := idx.Search("Pre-Rename File", 20)
	assert.NotEmpty(t, results,
		"content from renamed file should be accessible in the index after Create event fires")
}

// ---------------------------------------------------------------------------
// Chmod events

func TestWatcher_ChmodEvent_DoesNotTriggerReindex(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	populateIndex(t, store, idx)

	// Pre-populate the page.
	page, err := store.Write("chmod page", "Content of the chmod test page.")
	require.NoError(t, err)
	idx.Add(page)

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	defer w.Close()

	// Manually remove the page from the index so we can detect any
	// spurious re-indexing triggered by the chmod.
	idx.Remove("chmod page")
	require.False(t, indexContainsPage(idx, "Chmod Page"), "pre-condition: page must be removed before chmod")

	path := filepath.Join(dir, pages.NameToFilename("chmod page"))
	require.NoError(t, os.Chmod(path, 0o600))

	time.Sleep(debounceSettle)

	assert.False(t, indexContainsPage(idx, "Chmod Page"),
		"a chmod-only event must not re-index the page")
}

// ---------------------------------------------------------------------------
// Debouncing

func TestWatcher_Debounce_OnlyFinalStateIsIndexed(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	populateIndex(t, store, idx)

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	defer w.Close()

	path := filepath.Join(dir, pages.NameToFilename("debounce page"))

	// Write the file several times rapidly, each time within the 150 ms window,
	// resetting the debounce timer.  Only the final write should be indexed.
	for i := 1; i <= 5; i++ {
		content := fmt.Sprintf("# Debounce Page\nIntermediate version %d content.", i)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
		time.Sleep(30 * time.Millisecond)
	}

	// Final, settled write.
	require.NoError(t, os.WriteFile(path, []byte("# Debounce Page\nFinal settled version of the debounce page."), 0o644))

	time.Sleep(debounceSettle)

	assert.True(t, indexContainsPage(idx, "Debounce Page"),
		"page should be indexed once after the debounce window settles")
}

func TestWatcher_Debounce_RapidDeleteThenCreate_ProducesIndexedPage(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	populateIndex(t, store, idx)

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	defer w.Close()

	path := filepath.Join(dir, pages.NameToFilename("flicker page"))

	// Write, then quickly delete, then write again — simulates an editor that
	// uses a write-delete-rename dance to save atomically.
	require.NoError(t, os.WriteFile(path, []byte("# Flicker Page\nFirst write."), 0o644))
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, os.Remove(path))
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, os.WriteFile(path, []byte("# Flicker Page\nFinal settled write."), 0o644))

	time.Sleep(debounceSettle)

	assert.True(t, indexContainsPage(idx, "Flicker Page"),
		"page should be indexed after rapid delete-recreate sequence")
}

// ---------------------------------------------------------------------------
// Error handling

func TestWatcher_LoadFailure_SilentlyDropped(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	populateIndex(t, store, idx)

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	defer w.Close()

	// Write a file to trigger a Create event, then immediately delete it so
	// that when the debounce fires the file is gone and store.Load fails.
	path := filepath.Join(dir, pages.NameToFilename("ghost page"))
	require.NoError(t, os.WriteFile(path, []byte("# Ghost Page\nTransient content."), 0o644))
	// Delete within the debounce window so the file is gone when the timer fires.
	require.NoError(t, os.Remove(path))

	// Must not panic; the failed load should be silently dropped.
	time.Sleep(debounceSettle)

	assert.False(t, indexContainsPage(idx, "Ghost Page"),
		"a page whose file was deleted before load should not appear in the index")
}

func TestWatcher_RemoveNonExistentPage_IsNoOp(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	populateIndex(t, store, idx)

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	defer w.Close()

	// Write a file that was never indexed, then delete it.
	// The resulting Remove call on the index should be a no-op (not a crash).
	path := filepath.Join(dir, pages.NameToFilename("never indexed"))
	require.NoError(t, os.WriteFile(path, []byte("# Never Indexed\nContent."), 0o644))

	// Let the Create event be debounced and wait a beat, then delete.
	time.Sleep(debounceSettle)
	require.NoError(t, os.Remove(path))
	time.Sleep(debounceSettle)

	// No assertion beyond "did not panic".
}

// ---------------------------------------------------------------------------
// Self-triggered events (memento's own writes)

func TestWatcher_SelfTriggeredWrite_ReindexesWithoutError(t *testing.T) {
	dir, store, idx := newTestEnv(t)
	populateIndex(t, store, idx)

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	defer w.Close()

	// Use store.Write (memento's own write path) after the watcher is running.
	// The watcher will see the event and re-index; this should produce the same
	// result without errors and without duplicating the page.
	page, err := store.Write("self written", "Content written by memento itself.")
	require.NoError(t, err)
	idx.Add(page) // memento's tool also calls idx.Add directly

	time.Sleep(debounceSettle) // watcher will re-index the same page

	assert.True(t, indexContainsPage(idx, "Self Written"),
		"self-triggered event should leave the page correctly indexed")
}
