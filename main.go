package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/edgetools/memento/embed"
	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/edgetools/memento/tools"
	"github.com/edgetools/memento/watcher"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	contentDir := flag.String("content-dir", "", "Path to the directory containing markdown files (required)")
	autoCommit := flag.Bool("auto-commit", false, "Automatically git commit after every write operation")
	flag.Parse()

	if *contentDir == "" {
		fmt.Fprintln(os.Stderr, "error: --content-dir is required")
		os.Exit(1)
	}

	absDir, err := filepath.Abs(*contentDir)
	if err != nil {
		log.Fatalf("invalid --content-dir: %v", err)
	}

	if *autoCommit {
		// Verify that contentDir is inside a git repo.
		cmd := exec.Command("git", "rev-parse", "--git-dir")
		cmd.Dir = absDir
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "error: --auto-commit requires --content-dir to be inside a git repo: %s\n", out)
			os.Exit(1)
		}
	}

	model, err := embed.LoadModel()
	if err != nil {
		log.Fatalf("failed to load embedding model: %v", err)
	}

	store := pages.NewStore(absDir)
	cachePath := filepath.Join(absDir, ".memento-vectors")
	idx := index.NewIndex(model, cachePath)

	// Load embedding cache from the sidecar file (first-run: empty).
	cacheEntries, err := index.LoadCache(cachePath, model.ID(), model.SentexVersion(), model.Dimensions())
	if err != nil {
		log.Printf("warning: could not load embedding cache (will re-embed): %v", err)
		cacheEntries = nil
	}

	// Build a lookup map from normalized page name to cache entry.
	entryByName := make(map[string]index.CacheEntry, len(cacheEntries))
	for _, e := range cacheEntries {
		entryByName[pages.Normalize(e.PageName)] = e
	}

	// Scan and index pages, loading vectors from cache where available.
	for _, p := range store.Scan() {
		if cached, ok := entryByName[pages.Normalize(p.Name)]; ok {
			idx.AddFromCache(p, cached)
		} else {
			idx.Add(p)
		}
	}

	// Start filesystem watcher (non-fatal if unavailable).
	w, err := watcher.NewWatcher(absDir, store, idx)
	if err != nil {
		log.Printf("warning: filesystem watching unavailable: %v", err)
	} else {
		if err := w.Start(); err != nil {
			log.Printf("warning: filesystem watcher failed to start: %v", err)
		} else {
			defer w.Close()
		}
	}

	s := server.NewMCPServer("memento", "0.1.0", server.WithToolCapabilities(true))

	if *autoCommit {
		tools.RegisterAutoCommit(s, store, idx, absDir)
	} else {
		tools.Register(s, store, idx)
	}

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
