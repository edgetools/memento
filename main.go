package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/edgetools/memento/tools"
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

	store := pages.NewStore(absDir)
	idx := index.NewIndex()

	// Build index from existing pages.
	for _, p := range store.Scan() {
		idx.Add(p)
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
