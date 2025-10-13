package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	getGlobMatchesSyncMap = sync.Map{}

	// PathMatchCache stores PathMatch results to avoid redundant pattern matching.
	// Cache key: "pattern|name" -> match result (bool).
	// This cache is particularly effective during affected detection where the same
	// patterns are matched against the same files repeatedly in nested loops.
	pathMatchCache   = make(map[string]bool)
	pathMatchCacheMu sync.RWMutex
)

// GetGlobMatches tries to read and return the Glob matches content from the sync map if it exists in the map,
// otherwise it finds and returns all files matching the pattern, stores the files in the map and returns the files.
func GetGlobMatches(pattern string) ([]string, error) {
	defer perf.Track(nil, "utils.GetGlobMatches")()

	existingMatches, found := getGlobMatchesSyncMap.Load(pattern)
	if found && existingMatches != nil {
		return strings.Split(existingMatches.(string), ","), nil
	}

	pattern = filepath.ToSlash(pattern)
	base, cleanPattern := doublestar.SplitPattern(pattern)
	f := os.DirFS(base)

	matches, err := doublestar.Glob(f, cleanPattern)
	if err != nil {
		return nil, err
	}

	if matches == nil {
		return nil, fmt.Errorf("%w: '%s' ('%s' + '%s')", errUtils.ErrFailedToFindImport, pattern, base, cleanPattern)
	}

	var fullMatches []string
	for _, match := range matches {
		fullMatches = append(fullMatches, filepath.Join(base, match))
	}

	getGlobMatchesSyncMap.Store(pattern, strings.Join(fullMatches, ","))

	return fullMatches, nil
}

// PathMatch returns true if `name` matches the file name `pattern`.
// PathMatch will automatically
// use your system's path separator to split `name` and `pattern`. On systems
// where the path separator is `'\'`, escaping will be disabled.
//
// Note: this is meant as a drop-in replacement for filepath.Match(). It
// assumes that both `pattern` and `name` are using the system's path
// separator. If you can't be sure of that, use filepath.ToSlash() on both
// `pattern` and `name`, and then use the Match() function instead.
//
// Note: perf.Track() removed from this hot path to reduce overhead (called 2M+ times).
// Results are cached to avoid redundant pattern matching during affected detection.
func PathMatch(pattern, name string) (bool, error) {
	// Try cache first (read lock).
	cacheKey := pattern + "|" + name
	pathMatchCacheMu.RLock()
	result, found := pathMatchCache[cacheKey]
	pathMatchCacheMu.RUnlock()

	if found {
		return result, nil
	}

	// Cache miss - compute the result.
	match, err := doublestar.PathMatch(pattern, name)
	if err != nil {
		// Don't cache errors - they might be transient.
		return false, err
	}

	// Store result in cache (write lock).
	pathMatchCacheMu.Lock()
	pathMatchCache[cacheKey] = match
	pathMatchCacheMu.Unlock()

	return match, nil
}
