package watcher

import (
	"errors"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
)

const debounceWindow = 150 * time.Millisecond

// Watcher watches a content directory for .md file changes and updates the
// in-memory index accordingly. Debounces rapid sequences of events per file.
type Watcher struct {
	contentDir string
	store      *pages.Store
	idx        *index.Index
	fw         *fsnotify.Watcher

	mu      sync.Mutex
	started bool
	closed  bool
	quit    chan struct{}

	// debounce state: map from absolute path to pending timer
	timers   map[string]*time.Timer
	timersMu sync.Mutex
}

// NewWatcher creates a filesystem watcher on contentDir. Returns an error if
// fsnotify fails to initialize.
func NewWatcher(contentDir string, store *pages.Store, idx *index.Index) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := fw.Add(contentDir); err != nil {
		fw.Close()
		return nil, err
	}
	return &Watcher{
		contentDir: contentDir,
		store:      store,
		idx:        idx,
		fw:         fw,
		quit:       make(chan struct{}),
		timers:     make(map[string]*time.Timer),
	}, nil
}

// Start starts the watcher goroutine. Returns an error if already started.
func (w *Watcher) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return errors.New("watcher: already started")
	}
	if w.closed {
		return errors.New("watcher: already closed")
	}
	w.started = true
	go w.run()
	return nil
}

// Close stops the watcher goroutine and releases resources. Safe to call
// multiple times.
func (w *Watcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	close(w.quit)
	return w.fw.Close()
}

// run is the main event loop.
func (w *Watcher) run() {
	for {
		select {
		case <-w.quit:
			return
		case event, ok := <-w.fw.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.fw.Errors:
			if !ok {
				return
			}
			log.Printf("watcher: fsnotify error: %v", err)
		}
	}
}

// handleEvent filters and debounces a single fsnotify event.
func (w *Watcher) handleEvent(event fsnotify.Event) {
	path := event.Name
	base := filepath.Base(path)

	// Ignore the vector cache sidecar file.
	if base == ".memento-vectors" {
		return
	}

	// Ignore non-.md files.
	if filepath.Ext(base) != ".md" {
		return
	}

	// Ignore chmod-only events.
	if event.Op == fsnotify.Chmod {
		return
	}

	// Determine whether this is a delete/rename (file gone) or a create/write.
	isRemove := event.Op&(fsnotify.Remove|fsnotify.Rename) != 0

	w.debounce(path, isRemove)
}

// debounce resets (or creates) the per-path timer. When the timer fires the
// current state of the file is used to decide whether to add or remove.
func (w *Watcher) debounce(path string, isRemove bool) {
	w.timersMu.Lock()
	defer w.timersMu.Unlock()

	if t, ok := w.timers[path]; ok {
		t.Stop()
	}
	w.timers[path] = time.AfterFunc(debounceWindow, func() {
		w.timersMu.Lock()
		delete(w.timers, path)
		w.timersMu.Unlock()

		// Re-check current file existence at fire time (file may have changed
		// state during the debounce window).
		w.settle(path, isRemove)
	})
}

// settle performs the actual index update after the debounce window expires.
func (w *Watcher) settle(path string, lastEventWasRemove bool) {
	// The page name is always derived from the filename, consistent with
	// store.Scan and the CR6 spec. This ensures Remove and Add operate on the
	// correct keys even when the H1 title differs from the filename (e.g.
	// in-directory rename where the heading is unchanged).
	name := pages.FilenameToName(filepath.Base(path))

	// Try to load regardless of lastEventWasRemove — the file may have been
	// re-created during the debounce window (e.g. write-delete-rename dance).
	page, err := w.store.Load(name)
	if err != nil {
		// File is gone (or unreadable): remove from index if present.
		if lastEventWasRemove {
			w.idx.Remove(name)
		}
		// If it was a create/write but the file is now gone, silently drop.
		return
	}

	// Pin the page name to the filename-derived name. This keeps Remove and
	// Add operating on distinct keys for in-directory renames (where the H1
	// title could otherwise collide with the key being removed).
	page.Name = name

	w.idx.Add(page)
}
