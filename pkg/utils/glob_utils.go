package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
)

var (
	getGlobMatchesSyncMap = sync.Map{}
)

// GetGlobMatches tries to read and return the Glob matches content from the sync map if it exists in the map,
// otherwise it finds and returns all files matching the pattern, stores the files in the map and returns the files
func GetGlobMatches(pattern string) ([]string, error) {
	existingMatches, found := getGlobMatchesSyncMap.Load(pattern)
	if found && existingMatches != nil {
		return strings.Split(existingMatches.(string), ","), nil
	}

	// Normalize the pattern to use forward slashes
	pattern = filepath.ToSlash(pattern)

	base, cleanPattern := doublestar.SplitPattern(pattern)
	// Ensure base path is normalized for the OS
	base = filepath.FromSlash(base)
	f := os.DirFS(base)

	matches, err := doublestar.Glob(f, cleanPattern)
	if err != nil {
		return nil, err
	}

	if matches == nil {
		return nil, fmt.Errorf("failed to find a match for the import '%s' ('%s' + '%s')", pattern, base, cleanPattern)
	}

	var fullMatches []string
	for _, match := range matches {
		// First join with forward slashes for consistency
		fullPath := filepath.Join(base, match)
		// Then normalize for the current OS
		fullPath = filepath.FromSlash(fullPath)
		fullMatches = append(fullMatches, fullPath)
	}

	// Store normalized paths
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
func PathMatch(pattern, name string) (bool, error) {
	// Normalize both pattern and name to use forward slashes
	pattern = filepath.ToSlash(pattern)
	name = filepath.ToSlash(name)
	return doublestar.PathMatch(pattern, name)
}
