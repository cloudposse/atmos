// Package detect implements the filesystem-based pre-flight probes the
// atmos-pro skill relies on.
//
// Each probe inspects the repo's filesystem (via fs.FS so tests can feed
// synthetic trees) and returns a structured result. Probes are intentionally
// pure: they do not shell out to `gh`, `atmos`, or `git`. Network-dependent
// checks (`gh api repos/.../actions/permissions`,
// `atmos describe component github-oidc-provider`) stay as shell commands
// documented in references/starting-conditions.md — the skill's Bash tool
// executes those directly.
package detect

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Result is the structured output of a probe. Passed means the probe reached
// a definitive answer (see probe docs for the truth-value semantics). Details
// is a short human-readable summary. Hint is an actionable next-step for the
// skill or user.
type Result struct {
	// Name identifies the probe (e.g., "atmos-auth", "spacelift", "geodesic").
	Name string `json:"name"`
	// Detected reflects the probe's truth value. The semantics are probe-specific
	// and documented on each probe function.
	Detected bool `json:"detected"`
	// Details is a short human-readable summary (e.g., "3 stacks have Spacelift enabled").
	Details string `json:"details"`
	// Hint is a single-sentence actionable next step, or empty if none applies.
	Hint string `json:"hint,omitempty"`
	// Evidence lists the specific files or paths that drove the decision.
	Evidence []string `json:"evidence,omitempty"`
}

// ErrEmptyFS is returned when a probe is given a nil or empty filesystem.
var ErrEmptyFS = errors.New("atmospro/detect: filesystem is nil or empty")

// atmosYAMLCandidates are the file paths checked for a top-level `auth:` key.
var atmosYAMLCandidates = []string{
	"atmos.yaml",
	"atmos.yml",
	".atmos.yaml",
	".atmos.yml",
}

// atmosDirCandidates are directories whose *.yaml files are also scanned for
// top-level `auth:` keys. Matches the Atmos discovery convention.
var atmosDirCandidates = []string{
	"atmos.d",
	".atmos.d",
}

// geodesicMarkers are the filesystem signals that a repo is Geodesic-hosted.
// Any match promotes the repo to the Geodesic variant.
var geodesicMarkers = []struct {
	path    string
	content string // substring search within the file; empty means existence-only.
}{
	{"Dockerfile", "cloudposse/geodesic"},
	{"geodesic.mk", ""},
	{".envrc", "geodesic"},
	{"Makefile", "cloudposse/geodesic"},
}

// AtmosAuth reports whether the repo already has an `auth:` block configured.
// Detected=true means the skill must *not* retrofit Atmos Auth; it should
// generate standalone profiles and append to existing providers.
func AtmosAuth(fsys fs.FS) (Result, error) {
	defer perf.Track(nil, "atmospro/detect.AtmosAuth")()

	if fsys == nil {
		return Result{}, ErrEmptyFS
	}

	r := Result{Name: "atmos-auth"}

	// Check the primary atmos.yaml candidates.
	for _, name := range atmosYAMLCandidates {
		found, err := fileHasTopLevelKey(fsys, name, "auth")
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return r, fmt.Errorf("check %s: %w", name, err)
		}
		if found {
			r.Detected = true
			r.Evidence = append(r.Evidence, name)
		}
	}

	// Scan atmos.d/ directories for imported configs with auth:.
	for _, dir := range atmosDirCandidates {
		hits, err := scanDirForTopLevelKey(fsys, dir, "auth")
		if err != nil {
			return r, fmt.Errorf("scan %s: %w", dir, err)
		}
		r.Evidence = append(r.Evidence, hits...)
		if len(hits) > 0 {
			r.Detected = true
		}
	}

	if r.Detected {
		r.Details = fmt.Sprintf("Atmos Auth configured in %d file(s)", len(r.Evidence))
		r.Hint = "Add the github-oidc provider to the existing auth block via a patch file (atmos.d/atmos-pro.yaml) instead of editing the primary atmos.yaml."
	} else {
		r.Details = "no top-level auth: block detected"
		r.Hint = "Generate standalone profiles/github-{plan,apply}/atmos.yaml; do not retrofit auth: into atmos.yaml."
	}
	sort.Strings(r.Evidence)
	return r, nil
}

// Spacelift reports whether any stack manifest has
// `settings.spacelift.workspace_enabled: true`. Detected=true means the skill
// must flip workspace_enabled to false in the generated mixin.
//
// Evidence lists the stack files with the literal `workspace_enabled: true`
// line. This is a conservative text scan; it does NOT fully resolve inherited
// stack config. Good enough for a pre-flight warning.
func Spacelift(fsys fs.FS, stacksDir string) (Result, error) {
	defer perf.Track(nil, "atmospro/detect.Spacelift")()

	if fsys == nil {
		return Result{}, ErrEmptyFS
	}
	if stacksDir == "" {
		stacksDir = "stacks"
	}

	hits, err := walkStacksForSpacelift(fsys, stacksDir)
	if err != nil {
		return Result{Name: "spacelift"}, fmt.Errorf("walk %s: %w", stacksDir, err)
	}

	sort.Strings(hits)
	r := Result{Name: "spacelift", Evidence: hits, Detected: len(hits) > 0}
	if r.Detected {
		r.Details = fmt.Sprintf("Spacelift enabled in %d stack file(s)", len(hits))
		r.Hint = "The generated mixin will set settings.spacelift.workspace_enabled: false; flag the migration in the PR description."
	} else {
		r.Details = "no stack file enables Spacelift"
	}
	return r, nil
}

// walkStacksForSpacelift returns the stack files that literally enable
// Spacelift. Extracted so Spacelift() stays under the cyclomatic-complexity
// budget enforced by revive.
func walkStacksForSpacelift(fsys fs.FS, stacksDir string) ([]string, error) {
	var hits []string
	err := fs.WalkDir(fsys, stacksDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return walkErr
		}
		if d.IsDir() || !hasYAMLExtension(p) {
			return nil
		}
		contains, err := fileContainsLine(fsys, p, "workspace_enabled: true")
		if err != nil {
			return err
		}
		if contains {
			hits = append(hits, p)
		}
		return nil
	})
	return hits, err
}

// Geodesic reports whether the repo is Geodesic-hosted. Detected=true means
// the skill must add a Geodesic section to the generated docs/atmos-pro.md.
func Geodesic(fsys fs.FS) (Result, error) {
	defer perf.Track(nil, "atmospro/detect.Geodesic")()

	if fsys == nil {
		return Result{}, ErrEmptyFS
	}

	r := Result{Name: "geodesic"}

	for _, marker := range geodesicMarkers {
		data, err := fs.ReadFile(fsys, marker.path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return r, fmt.Errorf("read %s: %w", marker.path, err)
		}
		if marker.content == "" || strings.Contains(string(data), marker.content) {
			r.Detected = true
			r.Evidence = append(r.Evidence, marker.path)
		}
	}

	sort.Strings(r.Evidence)
	if r.Detected {
		r.Details = fmt.Sprintf("Geodesic signals found in %d file(s)", len(r.Evidence))
		r.Hint = "Generate the Geodesic section in docs/atmos-pro.md and document GITHUB_TOKEN passthrough for the Geodesic shell."
	} else {
		r.Details = "no Geodesic markers detected"
	}
	return r, nil
}

// All runs every filesystem probe and returns the results in a deterministic
// order. Callers use this for a single pre-flight sweep.
func All(fsys fs.FS, stacksDir string) ([]Result, error) {
	defer perf.Track(nil, "atmospro/detect.All")()

	var out []Result

	auth, err := AtmosAuth(fsys)
	if err != nil {
		return nil, err
	}
	out = append(out, auth)

	sl, err := Spacelift(fsys, stacksDir)
	if err != nil {
		return nil, err
	}
	out = append(out, sl)

	g, err := Geodesic(fsys)
	if err != nil {
		return nil, err
	}
	out = append(out, g)

	return out, nil
}

// hasYAMLExtension returns true for .yaml and .yml files. We intentionally
// do not trust extensionless files or case-variant extensions — Atmos itself
// only recognizes these two.
func hasYAMLExtension(p string) bool {
	ext := path.Ext(p)
	return ext == ".yaml" || ext == ".yml"
}

// fileHasTopLevelKey returns true when the file exists and its YAML has a
// top-level key matching the given name. It does a line-oriented scan rather
// than a full YAML parse so the detection is cheap and robust to comments.
func fileHasTopLevelKey(fsys fs.FS, filePath, key string) (bool, error) {
	f, err := fsys.Open(filePath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	prefix := key + ":"
	scanner := bufio.NewScanner(f)
	// A top-level key starts at column 0, optionally followed by ":" or ": value".
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, prefix) {
			return true, nil
		}
	}
	return false, scanner.Err()
}

// scanDirForTopLevelKey walks dir for *.yaml/*.yml files, checking each for a
// top-level key. Returns the sorted list of matching file paths. Missing dirs
// are not an error (they're expected in most repos).
func scanDirForTopLevelKey(fsys fs.FS, dir, key string) ([]string, error) {
	var hits []string
	err := fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !hasYAMLExtension(p) {
			return nil
		}
		found, err := fileHasTopLevelKey(fsys, p, key)
		if err != nil {
			return err
		}
		if found {
			hits = append(hits, p)
		}
		return nil
	})
	return hits, err
}

// fileContainsLine returns true when any line in the file (after trimming
// leading whitespace) contains the given substring.
func fileContainsLine(fsys fs.FS, filePath, needle string) (bool, error) {
	f, err := fsys.Open(filePath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, needle) {
			return true, nil
		}
	}
	return false, scanner.Err()
}
