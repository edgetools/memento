# Change Request: Filesystem Watcher

## Summary

Watch the content directory for external file changes and update the
in-memory index (BM25 and vector) without requiring a restart. This enables
memento to serve as a real-time bridge between concurrent Claude sessions
that share a brain.

---

## Changes

### New file: `watcher/watcher.go` (or `index/watcher.go`)

#### `Watcher` type

```go
type Watcher struct {
    // internal: fsnotify.Watcher, debounce state, references to store and index
}
```

#### `NewWatcher(contentDir string, store *pages.Store, idx *index.Index) (*Watcher, error)`

Creates a filesystem watcher on the content directory. Returns an error if
`fsnotify` fails to initialize (e.g. OS limits on inotify watches). The
caller should log a warning and continue without watching rather than
failing hard.

#### `(*Watcher) Start() error`

Starts the watcher goroutine. The goroutine reads filesystem events,
debounces them, and updates the index. Returns an error if the watcher is
already started.

#### `(*Watcher) Close() error`

Stops the watcher goroutine and releases resources. Safe to call multiple
times.

---

## Behavior Details

### Event handling

| Filesystem event | Action |
|---|---|
| `.md` file created | `store.Load(name)` then `idx.Add(page)` |
| `.md` file modified | `store.Load(name)` then `idx.Add(page)` (replaces existing) |
| `.md` file deleted | `idx.Remove(name)` |
| `.md` file renamed | `idx.Remove(oldName)`, then `store.Load(newName)` + `idx.Add(page)` |
| Non-`.md` file | Ignore |
| Directory events | Ignore (content directory is flat) |

The page name is derived from the filename using `pages.FilenameToName`,
consistent with how `store.Scan` works at startup.

### Debouncing

Text editors and tools often trigger multiple rapid filesystem events for
a single logical change (e.g. write temp file, rename, chmod). The watcher
debounces events per-file: after receiving an event for a file, it waits
100-200ms for additional events on the same file before acting. If more
events arrive during the window, the timer resets.

This prevents:
- Re-indexing a file mid-write
- Processing the intermediate states of a create-temp-then-rename dance
- Redundant re-indexing from editors that write twice (e.g. save + format)

### Self-triggered events

When memento's own MCP tools write a file, the filesystem watcher will also
see that event. The watcher does NOT attempt to suppress these — it simply
re-parses and re-indexes the file, which produces the same result as the
tool's own `idx.Add` call. The re-index is fast (single file parse) and
the simplicity of not tracking "own writes" is worth the negligible cost.

### Auto-commit interaction

When `-auto-commit` is enabled, a write operation produces:
1. The file write (triggers watcher event)
2. `git add` + `git commit` (does not trigger watcher for `.md` files since
   git operations don't modify the working tree files)

The debounce window ensures the watcher acts on the settled file state.

### Error handling

- If `store.Load` fails for a created/modified file (e.g. the file was
  deleted between the event and the load), the event is silently dropped.
- If `idx.Remove` is called for a page that isn't indexed, it's a no-op
  (consistent with existing `Index.Remove` behavior).
- If the fsnotify watcher encounters an error (e.g. too many watches),
  log the error but don't crash. The index will be stale for files changed
  after the error, recoverable by restarting memento.

### Startup sequence

The watcher starts AFTER the initial index build is complete. This ensures
the index has a consistent baseline before external changes are applied.
Events that arrive between process start and watcher initialization are
handled naturally by the initial `Scan()` + `Add()` loop.

---

## Non-Changes

- No changes to MCP tool schemas or output format.
- No changes to the search pipeline or ranking.
- No changes to BM25, vector index, or graph logic — the watcher uses
  the same `idx.Add`/`idx.Remove` interface as everything else.
- No subdirectory watching (content directory is flat by design).
- No watching for changes to the memento binary, config, or cache files.
