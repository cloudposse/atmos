package freshness

import (
	"errors"
	"fmt"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Globber resolves a glob pattern (relative to baseDir) to matching absolute file paths.
// Abstracted for testability (see mockGlobber in tests) -- the real implementation wraps
// pkg/filesystem.GetGlobMatches, the same doublestar-based, LRU-cached glob engine already
// used elsewhere in Atmos, rather than pkg/filematch (used by `atmos validate schema`'s
// `matches:`), because filematch.MatchFiles resolves against the process's current working
// directory with no baseDir parameter, and freshness must resolve relative to the step's own
// working directory, which can differ from process CWD.
type Globber interface {
	Glob(baseDir, pattern string) ([]string, error)
}

type defaultGlobber struct{}

// NewGlobber returns the real, production Globber.
func NewGlobber() Globber {
	defer perf.Track(nil, "freshness.NewGlobber")()

	return defaultGlobber{}
}

func (defaultGlobber) Glob(baseDir, pattern string) ([]string, error) {
	defer perf.Track(nil, "freshness.Globber.Glob")()

	full := pattern
	if !filepath.IsAbs(full) {
		full = filepath.Join(baseDir, pattern)
	}
	matches, err := filesystem.GetGlobMatches(full)
	if err != nil {
		// A missing base directory (e.g. sources: point at a directory that hasn't been
		// created yet) means "nothing matched," not a hard error.
		if errors.Is(err, errUtils.ErrFailedToFindImport) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("%w: pattern %q in %q: %w", ErrGlobInvalid, pattern, baseDir, err)
	}
	return matches, nil
}

// globAll resolves every pattern against baseDir via g, returning the combined, sorted-by-
// caller-if-needed match list. Duplicate matches (two patterns matching the same file) are
// deduplicated.
func globAll(g Globber, baseDir string, patterns []string) ([]string, error) {
	seen := make(map[string]struct{})
	var all []string
	for _, pattern := range patterns {
		matches, err := g.Glob(baseDir, pattern)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			if _, ok := seen[m]; ok {
				continue
			}
			seen[m] = struct{}{}
			all = append(all, m)
		}
	}
	return all, nil
}
