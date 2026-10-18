package doctor

import (
	"fmt"
	"path/filepath"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/validation"
)

// runtimeBasePathSource marks a base path supplied by a flag or environment variable, which makes
// dot-prefixed values relative to the working directory instead of the configuration directory.
const runtimeBasePathSource = "runtime"

// configLayout is the non-parsing result of locating the Atmos configuration for one invocation.
type configLayout struct {
	// sources are the project configuration files in merge order, lowest precedence first.
	sources []source
	// ambient are existing home and system configuration files, which are merged at runtime
	// but are not part of the project.
	ambient []string
	// profiles are the files of the active profiles, highest precedence.
	profiles []source
	// missingProfiles are active profiles that have no directory.
	missingProfiles []string
	// configDir is the directory of the highest-precedence base configuration file.
	configDir string
	// root is the project root implied by where the configuration was discovered.
	root string
	// project is true when any project configuration was selected or discovered.
	project bool
}

// resolveConfigLayout locates configuration candidates exactly as the Atmos loader would, without
// parsing them. The active profiles and their base path are supplied once the base configuration is known.
func resolveConfigLayout(o *Options, profiles []string, profilesBasePath, declaringDir string) (*configLayout, error) {
	options := []cfg.CandidateOption{cfg.WithWorkingDir(o.Root), cfg.WithConfigFiles(o.ConfigFiles...), cfg.WithConfigDirs(o.ConfigDirs...)}
	if len(profiles) > 0 {
		options = append(options, cfg.WithProfiles(profiles...), cfg.WithProfilesBasePath(profilesBasePath, declaringDir))
	}
	candidates, err := cfg.ResolveConfigCandidates(options...)
	if err != nil {
		return nil, err
	}
	layout := &configLayout{root: o.Root}
	for i := range candidates {
		layout.add(o, &candidates[i])
	}
	return layout, nil
}

// add classifies one candidate. Ambient configuration never becomes a source because it is not
// project input; missing explicit selections become sources so the read reports them.
func (l *configLayout) add(o *Options, c *cfg.ConfigCandidate) {
	path := canonicalPath(c.Path)
	switch c.Kind {
	case cfg.ConfigCandidateAmbient:
		if c.Exists {
			l.ambient = append(l.ambient, path)
		}
		return
	case cfg.ConfigCandidateProfile:
		if c.Exists {
			l.profiles = append(l.profiles, source{path, documentConfig})
		} else {
			l.missingProfiles = append(l.missingProfiles, c.Profile)
		}
		return
	case cfg.ConfigCandidateBase:
		l.project = true
		if c.Exists {
			l.configDir = filepath.Dir(path)
			l.root = discoveredRoot(o, c.Source, l.configDir, l.root)
		}
	}
	l.project = true
	l.sources = append(l.sources, source{path, documentConfig})
}

// discoveredRoot moves the project root to the configuration directory only when the
// configuration was found above the working directory (parent directory, Git root, or
// ATMOS_CLI_CONFIG_PATH). Explicit --config selections keep the invocation directory as the root.
func discoveredRoot(o *Options, candidateSource, configDir, current string) string {
	switch candidateSource {
	case "parent directory", "git root", cfg.AtmosCliConfigPathEnvVar:
		return configDir
	case "working directory":
		return o.Root
	}
	return current
}

// canonicalPath resolves symlinks in a path's directory while preserving a symlinked leaf,
// because symlinked sources are reviewed rather than followed.
func canonicalPath(path string) string {
	dir, name := filepath.Split(path)
	if resolved, err := filepath.EvalSymlinks(filepath.Clean(dir)); err == nil {
		return filepath.Join(resolved, name)
	}
	return filepath.Clean(path)
}

// projectBasePath resolves base_path with the same function the Atmos loader uses, so relative
// values, the runtime override, and the Git-root default agree with `atmos` itself.
func projectBasePath(o *Options, p *project, configDir string) (string, error) {
	basePath, source := stringAt(p.config, keyBasePath), ""
	runtime := o.BasePath
	if runtime == "" {
		runtime = cfg.ConfigSelectionFromEnv().BasePath
	}
	if runtime != "" {
		basePath, source = runtime, runtimeBasePathSource
	}
	base, err := cfg.ResolveBasePath(o.Root, basePath, configDir, source)
	if err != nil {
		return "", err
	}
	return canonicalPath(base), nil
}

// rerootProject moves the project root and rebases affected-file selections that were relative to
// the previous root.
func rerootProject(p *project, o *Options, root string) {
	if root == "" || root == p.root {
		return
	}
	rebased := make([]string, 0, len(o.Selected))
	for _, path := range o.Selected {
		rel, err := filepath.Rel(root, filepath.Join(p.root, filepath.FromSlash(path)))
		if err != nil {
			continue // Different volume; the path cannot be inside the new root.
		}
		rebased = append(rebased, filepath.ToSlash(rel))
	}
	o.Selected = rebased
	p.root = root
	o.Root = root
}

// noProjectError explains how to select a project when none was found.
func noProjectError(o *Options) error {
	return fmt.Errorf("%w in %s: run atmos doctor from the project directory, or select a configuration with --config or ATMOS_CLI_CONFIG_PATH", ErrNoConfiguration, o.Root)
}

// resolveProfiles resolves the files of the active profiles once the base configuration declares
// where profiles live. Only profile candidates are returned; the base layout is already loaded.
func resolveProfiles(o *Options, p *project, base *configLayout) (*configLayout, error) {
	p.profiles = activeProfiles(o, p)
	if len(p.profiles) == 0 {
		return &configLayout{}, nil
	}
	layout, err := resolveConfigLayout(o, p.profiles, stringAt(p.config, "profiles", keyBasePath), base.configDir)
	if err != nil {
		return nil, err
	}
	return &configLayout{sources: layout.profiles, missingProfiles: layout.missingProfiles}, nil
}

// activeProfiles chooses the profiles like the loader: --profile or ATMOS_PROFILE win, and the base
// configuration's profiles.default applies only when neither is given.
func activeProfiles(o *Options, p *project) []string {
	if len(o.Profiles) > 0 {
		return o.Profiles
	}
	if name := strings.TrimSpace(stringAt(p.config, "profiles", "default")); name != "" {
		return []string{name}
	}
	return nil
}

// missingProfileFinding reports an active profile with no directory, which the real CLI rejects.
func missingProfileFinding(name string) *Finding {
	return &Finding{RuleID: ruleImports, Category: categoryValidation, Severity: validation.SeverityError, Message: "Active profile was not found: " + name, Status: statusManual, Remediation: "Create the profile directory or select an existing profile with --profile"}
}

// noteOutsideSources collapses several configuration files that live outside the project, such as
// the Git root's atmos.d when the project is nested, into one notice per directory. The runtime
// merges them, so verification is incomplete, but doctor never reads or edits them. A lone file
// stays in the list so the reader reports it individually.
func noteOutsideSources(p *project, o *Options, r *Report, sources []source) []source {
	counts := map[string]int{}
	var order []string
	for _, s := range sources {
		if within(p.root, s.path) {
			continue
		}
		dir := filepath.Dir(s.path)
		if counts[dir] == 0 {
			order = append(order, dir)
		}
		counts[dir]++
	}
	for _, dir := range order {
		if counts[dir] > 1 {
			p.limit(limitConfig, "", 0, fmt.Sprintf("%d configuration files in %s are merged at runtime but are outside the project", counts[dir], dir))
			addNotice(o, r, incompleteNotice("", fmt.Sprintf("%d configuration files in %s are merged at runtime but are outside the project; they will not be edited", counts[dir], dir)))
		}
	}
	kept := make([]source, 0, len(sources))
	for _, s := range sources {
		if within(p.root, s.path) || counts[filepath.Dir(s.path)] == 1 {
			kept = append(kept, s)
		}
	}
	return kept
}
