package engine

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// discoveryPaths walks only the literal prefixes of included patterns and prunes
// excluded directories. Paths select entry points; HCL references are resolved later.
func discoveryPaths(root string, includes, excludes []string) ([]string, []string, error) {
	if len(includes) == 0 {
		includes = []string{"**"}
	}
	for _, pattern := range append(slices.Clone(includes), excludes...) {
		if pattern == "" || filepath.IsAbs(pattern) || strings.Contains(pattern, "\\") || !doublestar.ValidatePattern(pattern) {
			return nil, nil, fmt.Errorf("invalid discovery path pattern: %q", pattern)
		}
		for _, part := range strings.Split(pattern, "/") {
			if part == ".." {
				return nil, nil, fmt.Errorf("discovery paths must stay within the Atmos base path: %q", pattern)
			}
		}
	}
	matches := func(patterns []string, path string) bool {
		for _, pattern := range patterns {
			if ok, _ := doublestar.Match(pattern, path); ok {
				return true
			}
		}
		return false
	}
	seen := map[string]bool{}
	var stacks, units []string
	for _, pattern := range includes {
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
			return nil, nil, fmt.Errorf("discovery root is outside the Atmos base path or unavailable: %s", start)
		}
		err = filepath.WalkDir(start, func(path string, entry fs.DirEntry, walkErr error) error {
			if os.IsNotExist(walkErr) && path == start {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if matches(excludes, rel) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				if path != root && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules") {
					return filepath.SkipDir
				}
				return nil
			}
			if seen[path] || !matches(includes, rel) {
				return nil
			}
			seen[path] = true
			switch entry.Name() {
			case "terragrunt.stack.hcl":
				stacks = append(stacks, path)
			case "terragrunt.hcl":
				units = append(units, path)
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	slices.Sort(stacks)
	slices.Sort(units)
	return stacks, units, nil
}
