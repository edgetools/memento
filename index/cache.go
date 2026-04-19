package index

import (
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// cacheVersion is bumped when the on-disk format changes in a backward-incompatible way.
const cacheVersion = 1

// CacheEntry holds the cached embeddings for a single page.
type CacheEntry struct {
	PageName    string
	ContentHash string
	Chunks      []CachedChunk
}

// CachedChunk holds the embedding vector and line range for a single chunk.
type CachedChunk struct {
	StartLine int
	EndLine   int
	Vector    []float32
}

// cacheHeader is written as the first gob value in the cache file.
type cacheHeader struct {
	ModelID       string
	SentexVersion string
	Dimensions    int
	Version       int
}

// SaveCache writes entries to path atomically (temp file + rename).
// Every save is a full rewrite of the cache.
func SaveCache(path string, entries []CacheEntry, modelID, sentexVersion string, dims int) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".memento-vectors-tmp-*")
	if err != nil {
		return fmt.Errorf("cache: create temp file: %w", err)
	}
	tmpName := tmp.Name()

	// Always clean up the temp file on failure.
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	enc := gob.NewEncoder(tmp)

	hdr := cacheHeader{
		ModelID:       modelID,
		SentexVersion: sentexVersion,
		Dimensions:    dims,
		Version:       cacheVersion,
	}
	if err := enc.Encode(hdr); err != nil {
		return fmt.Errorf("cache: encode header: %w", err)
	}
	if err := enc.Encode(entries); err != nil {
		return fmt.Errorf("cache: encode entries: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cache: close temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("cache: rename to final path: %w", err)
	}
	ok = true
	return nil
}

// LoadCache reads the cache from path. Returns empty slice (no error) when:
//   - the file does not exist (first run)
//   - any header field mismatches (stale cache — model/version/dims changed)
//
// Returns nil + error when the file exists but cannot be decoded (corrupt).
func LoadCache(path string, modelID, sentexVersion string, dims int) ([]CacheEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("cache: open: %w", err)
	}
	defer f.Close()

	dec := gob.NewDecoder(f)

	var hdr cacheHeader
	if err := dec.Decode(&hdr); err != nil {
		return nil, fmt.Errorf("cache: decode header: %w", err)
	}

	// Stale cache — return empty without error.
	if hdr.ModelID != modelID || hdr.SentexVersion != sentexVersion || hdr.Dimensions != dims || hdr.Version != cacheVersion {
		return nil, nil
	}

	var entries []CacheEntry
	if err := dec.Decode(&entries); err != nil {
		return nil, fmt.Errorf("cache: decode entries: %w", err)
	}

	return entries, nil
}
