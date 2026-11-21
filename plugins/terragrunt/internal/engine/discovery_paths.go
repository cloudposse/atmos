package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

var (
	errDiscoveryPattern   = errors.New("invalid discovery path pattern")
	errDiscoveryTraversal = errors.New("discovery paths must stay within the Atmos base path")
	errDiscoveryRoot      = errors.New("discovery root is outside the Atmos base path or unavailable")
)

// discoveryPaths walks only the literal prefixes of included patterns and prunes
// excluded directories. Paths select entry points; HCL references are resolved later.
func discoveryPaths(root string, includes, excludes []string) ([]string, []string, error) {
	if len(includes) == 0 {
		includes = []string{"**"}
	}
	if err := validateDiscoveryPatterns(append(slices.Clone(includes), excludes...)); err != nil {
		return nil, nil, err
	}
	walk := discoveryPathWalk{root: root, includes: includes, excludes: excludes, seen: map[string]bool{}}
	for _, pattern := range includes {
		start, err := discoveryStart(root, pattern)
		if err != nil {
			return nil, nil, err
		}
		walk.start = start
		if err := filepath.WalkDir(start, walk.visit); err != nil {
			return nil, nil, err
		}
	}
	slices.Sort(walk.stacks)
	slices.Sort(walk.units)
	return walk.stacks, walk.units, nil
}

func validateDiscoveryPatterns(patterns []string) error {
	for _, pattern := range patterns {
		if pattern == "" || filepath.IsAbs(pattern) || strings.Contains(pattern, "\\") || !doublestar.ValidatePattern(pattern) {
			return fmt.Errorf("%w: %q", errDiscoveryPattern, pattern)
		}
		for _, part := range strings.Split(pattern, "/") {
			if part == ".." {
				return fmt.Errorf("%w: %q", errDiscoveryTraversal, pattern)
			}
		}
	}
	return nil
}

func discoveryStart(root, pattern string) (string, error) {
	prefix := pattern
	if index := strings.IndexAny(pattern, "*?[{"); index >= 0 {
		prefix = pattern[:index]
		if !strings.HasSuffix(prefix, "/") {
			prefix = filepath.Dir(prefix)
		}
	}
	start := filepath.Join(root, filepath.FromSlash(prefix))
	// Do not follow a configured root symlink outside the project.
	resolved, err := canonicalPath(start)
	if err != nil || !within(resolved, root) {
		return "", fmt.Errorf("%w: %s", errDiscoveryRoot, start)
	}
	return start, nil
}

func matchesDiscoveryPaths(patterns []string, path string) bool {
	for _, pattern := range patterns {
		if ok, _ := doublestar.Match(pattern, path); ok {
			return true
		}
	}
	return false
}

// discoveryPathWalk shares duplicate tracking across independently scoped roots.
type discoveryPathWalk struct {
	root, start        string
	includes, excludes []string
	seen               map[string]bool
	stacks, units      []string
}

func (w *discoveryPathWalk) visit(path string, entry fs.DirEntry, walkErr error) error {
	if os.IsNotExist(walkErr) && path == w.start {
		return nil
	}
	if walkErr != nil {
		return walkErr
	}
	rel, _ := filepath.Rel(w.root, path)
	rel = filepath.ToSlash(rel)
	if matchesDiscoveryPaths(w.excludes, rel) {
		if entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	if entry.IsDir() {
		return w.visitDirectory(path, entry)
	}
	if w.seen[path] || !matchesDiscoveryPaths(w.includes, rel) {
		return nil
	}
	w.record(path, entry.Name())
	return nil
}

func (w *discoveryPathWalk) record(path, name string) {
	w.seen[path] = true
	switch name {
	case stackFile:
		w.stacks = append(w.stacks, path)
	case unitFile:
		w.units = append(w.units, path)
	}
}

func (w *discoveryPathWalk) visitDirectory(path string, entry fs.DirEntry) error {
	if path != w.root && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules") {
		return filepath.SkipDir
	}
	return nil
}
