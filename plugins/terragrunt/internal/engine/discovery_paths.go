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

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// pathScan is what a walk of the discovery scope found. Stacks and Units are absolute
// paths of terragrunt.stack.hcl and terragrunt.hcl files.
type pathScan struct {
	Stacks, Units []string
	Warnings      []model.Warning
}

// normalizePattern rewrites a configured pattern into one canonical, slash-separated form
// so included_paths and excluded_paths treat `live`, `live/`, `./live` and `live/**` alike.
func normalizePattern(pattern string) string {
	for strings.HasPrefix(pattern, "./") {
		pattern = strings.TrimPrefix(pattern, "./")
	}
	pattern = strings.TrimRight(pattern, "/")
	if pattern == "" || pattern == "." {
		return "**"
	}
	return pattern
}

// selects reports whether a pattern selects a project-relative path. A pattern selects
// what it matches and everything below it, so a directory name needs no `/**` suffix.
func selects(pattern, rel string) bool {
	if ok, _ := doublestar.Match(pattern, rel); ok {
		return true
	}
	ok, _ := doublestar.Match(pattern+"/**", rel)
	return ok
}

func selectsAny(patterns []string, rel string) bool {
	return slices.ContainsFunc(patterns, func(pattern string) bool { return selects(pattern, rel) })
}

// validatePatterns normalizes and checks both lists, naming the offending key and pattern.
func validatePatterns(includes, excludes []string) ([]string, []string, error) {
	normalize := func(key string, patterns []string) ([]string, error) {
		result := make([]string, 0, len(patterns))
		for _, pattern := range patterns {
			if pattern == "" || filepath.IsAbs(pattern) || strings.Contains(pattern, "\\") || !doublestar.ValidatePattern(pattern) {
				return nil, fmt.Errorf("%w: %s pattern %q", errInvalidDiscoveryPattern, key, pattern)
			}
			for _, part := range strings.Split(pattern, "/") {
				if part == ".." {
					return nil, fmt.Errorf("%w: %s patterns must stay within the Atmos base path: %q", errInvalidDiscoveryPattern, key, pattern)
				}
			}
			result = append(result, normalizePattern(pattern))
		}
		return result, nil
	}
	includes, err := normalize("included_paths", includes)
	if err != nil {
		return nil, nil, err
	}
	excludes, err = normalize("excluded_paths", excludes)
	return includes, excludes, err
}

// walkStart is the directory or file a pattern's walk begins at: its literal prefix.
func walkStart(root, pattern string) string {
	prefix := pattern
	if index := strings.IndexAny(pattern, "*?[{"); index >= 0 {
		prefix = pattern[:index]
		if !strings.HasSuffix(prefix, "/") {
			prefix = filepath.Dir(prefix)
		}
	}
	return filepath.Join(root, filepath.FromSlash(prefix))
}

// discoveryPaths walks only the literal prefixes of included patterns and prunes
// excluded directories. Paths select entry points; HCL references are resolved later.
//
// Like upstream Terragrunt, directory symlinks are not followed and a symlinked
// terragrunt.hcl is a unit at the place the link lives. A skipped link is reported.
func discoveryPaths(root string, includes, excludes []string) (pathScan, error) {
	if len(includes) == 0 {
		includes = []string{"**"}
	}
	includes, excludes, err := validatePatterns(includes, excludes)
	if err != nil {
		return pathScan{}, err
	}
	walker := &discoveryWalker{root: root, includes: includes, excludes: excludes, seen: map[string]bool{}, matched: map[string]int{}}
	for _, pattern := range includes {
		if err := walker.walk(pattern); err != nil {
			return pathScan{}, err
		}
	}
	scan := walker.scan
	matched := walker.matched
	slices.Sort(scan.Stacks)
	slices.Sort(scan.Units)
	scan.Warnings = append(scan.Warnings, unmatchedWarnings(includes, matched)...)
	slices.SortFunc(scan.Warnings, func(a, b model.Warning) int { return strings.Compare(a.Code+a.Path+a.Message, b.Code+b.Path+b.Message) })
	scan.Warnings = slices.Compact(scan.Warnings)
	return scan, nil
}

// isTerragruntFile reports whether a file name is a Terragrunt unit or stack configuration.
func isTerragruntFile(name string) bool {
	return name == "terragrunt.hcl" || name == "terragrunt.stack.hcl"
}

// unmatchedWarnings names included_paths patterns that selected no Terragrunt file.
// Counting happens per configuration file, so a pattern that matches only directories,
// or a name that differs in case, is reported instead of silently producing no units.
func unmatchedWarnings(includes []string, matched map[string]int) []model.Warning {
	var warnings []model.Warning
	for _, pattern := range slices.Compact(slices.Sorted(slices.Values(includes))) {
		if matched[pattern] == 0 {
			warnings = append(warnings, model.Warning{
				Code: model.WarningPatternUnmatched, Path: pattern,
				Message: fmt.Sprintf("included_paths pattern %q matched no terragrunt.hcl or terragrunt.stack.hcl (patterns are relative to the Atmos base path and select the path and everything below it)", pattern),
			})
		}
	}
	return warnings
}

func symlinkWarning(rel string) model.Warning {
	return model.Warning{
		Code: model.WarningSymlinkedDirectory, Path: rel,
		Message: fmt.Sprintf("symlinked directory %s was not followed, so units below it are not discovered; like Terragrunt, Atmos follows only a symlinked terragrunt.hcl or terragrunt.stack.hcl file", rel),
	}
}

var (
	errInvalidDiscoveryPattern = errors.New("invalid discovery pattern")
	errDiscoveryRoot           = errors.New("discovery root is outside the Atmos base path or unavailable")
)

type discoveryWalker struct {
	root               string
	includes, excludes []string
	seen               map[string]bool
	matched            map[string]int
	scan               pathScan
}

func (w *discoveryWalker) walk(pattern string) error {
	start := walkStart(w.root, pattern)
	resolved, err := canonicalPath(start)
	if err != nil || !within(resolved, w.root) {
		return fmt.Errorf("%w: %s", errDiscoveryRoot, start)
	}
	// WalkDir does not follow links below its start, but it does open a start that sits beneath a
	// linked directory, so a symlinked ancestor is reported and skipped like any other linked directory.
	if link, linked := symlinkedAncestor(w.root, start); linked {
		w.scan.Warnings = append(w.scan.Warnings, symlinkWarning(link))
		return nil
	}
	// A scoped walk starts below the project root, so it never visits the directories above its
	// start; they get the same pruning a root-wide scan applies, such as .terragrunt-cache.
	if prunedAncestor(w.root, start) {
		return nil
	}
	return filepath.WalkDir(start, func(path string, entry fs.DirEntry, walkErr error) error {
		if os.IsNotExist(walkErr) && path == start {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		return w.visit(path, entry)
	})
}

// prunedDirectory reports a directory discovery never descends into: hidden directories, such as
// .terragrunt-cache and .terragrunt-stack, and node_modules.
func prunedDirectory(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules"
}

// prunedAncestor reports whether a directory between root and start, inclusive of start, is pruned.
func prunedAncestor(root, start string) bool {
	rel, err := filepath.Rel(root, start)
	if err != nil || rel == "." {
		return false
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if prunedDirectory(part) {
			return true
		}
	}
	return false
}

// symlinkedAncestor reports the first directory symlink between root and start, inclusive of start,
// as a root-relative slash path. A symlinked configuration file at the end is not a directory link.
func symlinkedAncestor(root, start string) (string, bool) {
	rel, err := filepath.Rel(root, start)
	if err != nil || rel == "." {
		return "", false
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", false
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			continue
		}
		if target, err := os.Stat(current); err == nil && target.IsDir() {
			link, _ := filepath.Rel(root, current)
			return filepath.ToSlash(link), true
		}
	}
	return "", false
}

func (w *discoveryWalker) visit(path string, entry fs.DirEntry) error {
	rel, _ := filepath.Rel(w.root, path)
	rel = filepath.ToSlash(rel)
	if selectsAny(w.excludes, rel) {
		if entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	if entry.Type()&fs.ModeSymlink != 0 {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			w.scan.Warnings = append(w.scan.Warnings, symlinkWarning(rel))
			return nil
		}
	}
	if entry.IsDir() {
		if path != w.root && prunedDirectory(entry.Name()) {
			return filepath.SkipDir
		}
		return nil
	}
	w.record(path, rel, entry.Name())
	return nil
}

func (w *discoveryWalker) record(path, rel, name string) {
	// Only configuration files count toward matched-pattern warnings.
	if !isTerragruntFile(name) || !selectsAny(w.includes, rel) {
		return
	}
	for _, candidate := range w.includes {
		if selects(candidate, rel) {
			w.matched[candidate]++
		}
	}
	if w.seen[path] {
		return
	}
	w.seen[path] = true
	switch name {
	case "terragrunt.stack.hcl":
		w.scan.Stacks = append(w.scan.Stacks, path)
	case "terragrunt.hcl":
		w.scan.Units = append(w.scan.Units, path)
	}
}

func checkIdentityCollisions(root string, inventory []model.DiscoveredUnit) error {
	identities := map[string]string{}
	paths := map[string]string{}
	for i := range inventory {
		unit := &inventory[i]
		key := unit.Stack + "\x00" + unit.Name
		if previous, exists := identities[key]; exists {
			return &model.Problem{
				Code: model.CodeEvaluationError, Summary: "Two Terragrunt units resolve to the same component", Detail: fmt.Sprintf("%s/%s is produced by both %s and %s.", unit.Stack, unit.Name, filepath.ToSlash(mustRel(root, previous)), filepath.ToSlash(mustRel(root, unit.Config.Path))), Safe: true,
				Hints: []string{"Remove one of the units, a symlink that repeats a unit, or an overlapping `included_paths` entry."},
			}
		}
		identities[key] = unit.Config.Path
		if previous, exists := paths[unit.Config.Path]; exists {
			return &model.Problem{Code: model.CodeEvaluationError, Summary: "Two components resolve to the same Terragrunt unit", Detail: fmt.Sprintf("%s and %s/%s both use %s.", previous, unit.Stack, unit.Name, filepath.ToSlash(mustRel(root, unit.Config.Path))), Safe: true}
		}
		paths[unit.Config.Path] = unit.Stack + "/" + unit.Name
	}
	return nil
}
