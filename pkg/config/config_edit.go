package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	goyaml "go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ErrNoEditableConfig is returned when an editable atmos.yaml file cannot be located.
var ErrNoEditableConfig = errors.New("could not locate an editable atmos.yaml; pass --config to target a specific file")

// ErrAmbiguousConfigFile is returned when a config-editing command (config set/delete/format,
// mcp config add/remove) is given more than one --config file. Unlike `config get`, which reads
// the fully-merged effective value across every --config file, these commands mutate exactly one
// concrete file on disk -- silently picking the first (or last) would edit a file the user may
// not have intended, while the actual effective config (what every other atmos command uses)
// stays unchanged whenever a later file also sets the same key (cloudposse/atmos#2867).
var ErrAmbiguousConfigFile = errors.New("multiple --config files given; specify exactly one file to edit")

// ResolveConfigOverride validates that cfgFiles names at most one file, returning it (or "" if
// none), since config set/delete/format and mcp config add/remove edit a single concrete file
// and cannot safely guess which of several --config files to target.
func ResolveConfigOverride(cfgFiles []string) (string, error) {
	if len(cfgFiles) > 1 {
		return "", fmt.Errorf("%w: %s", ErrAmbiguousConfigFile, strings.Join(cfgFiles, ", "))
	}
	if len(cfgFiles) == 1 {
		return cfgFiles[0], nil
	}
	return "", nil
}

// configFileCandidates lists the config file names to probe in a directory, in
// precedence order (atmos.yaml before the dotfile variant).
var configFileCandidates = []string{AtmosConfigFileName, DotAtmosConfigFileName}

// ResolveEditableConfigFile returns the path to the atmos.yaml file that config
// edits should target. Because atmos.yaml can be merged from several locations,
// edits operate on a single concrete file chosen by this precedence:
//
//  1. an explicit override (the --config flag or ATMOS_CLI_CONFIG_PATH);
//  2. atmos.yaml / .atmos.yaml in the current working directory;
//  3. atmos.yaml / .atmos.yaml at the git repository root.
//
// It returns ErrNoEditableConfig if none of these exist, so callers can prompt
// the user to create one or pass --config explicitly.
func ResolveEditableConfigFile(atmosConfig *schema.AtmosConfiguration, override string) (string, error) {
	defer perf.Track(atmosConfig, "config.ResolveEditableConfigFile")()

	if override != "" {
		return resolveOverridePath(override)
	}

	cwd, err := os.Getwd()
	if err == nil {
		path, ok, probeErr := firstExistingConfig(cwd)
		if probeErr != nil {
			return "", probeErr
		}
		if ok {
			return path, nil
		}
	}

	if gitRoot, gitErr := u.ProcessTagGitRoot("!repo-root ."); gitErr == nil && gitRoot != "" {
		path, ok, probeErr := firstExistingConfig(gitRoot)
		if probeErr != nil {
			return "", probeErr
		}
		if ok {
			return path, nil
		}
	}

	return "", ErrNoEditableConfig
}

// configImportDirCandidates lists the default-import directory names probed for
// config fragments, matching the directories mergeDefaultImports auto-discovers.
var configImportDirCandidates = []string{AtmosDefaultImportsDirName, DotAtmosDefaultImportsDirName}

// ResolveEditableConfigFileForSection returns the config file that edits to the
// given top-level section (e.g. "mcp") should target. It extends
// ResolveEditableConfigFile with fragment-awareness: when no explicit override is
// given and an auto-discovered `atmos.d/`/`.atmos.d/` fragment already declares the
// section, that fragment is edited instead of the root atmos.yaml, so a project
// that keeps modular config in fragments is not silently split across two files
// (cloudposse/atmos#3269). Precedence:
//
//  1. an explicit override (the --config flag or ATMOS_CLI_CONFIG_PATH);
//  2. an auto-discovered fragment that already declares the section;
//  3. the root atmos.yaml / .atmos.yaml (via ResolveEditableConfigFile).
func ResolveEditableConfigFileForSection(atmosConfig *schema.AtmosConfiguration, override, section string) (string, error) {
	defer perf.Track(atmosConfig, "config.ResolveEditableConfigFileForSection")()

	if override != "" {
		return resolveOverridePath(override)
	}
	if fragment, ok := fragmentDeclaringSection(section); ok {
		return fragment, nil
	}
	return ResolveEditableConfigFile(atmosConfig, "")
}

// fragmentDeclaringSection returns the first auto-discovered config fragment that
// declares the given top-level section, searching the current working directory
// first and the git repository root second -- mirroring how mergeDefaultImports
// discovers atmos.d/.atmos.d. Returns false when no fragment declares the
// section, so the caller falls back to the root atmos.yaml.
func fragmentDeclaringSection(section string) (string, bool) {
	for _, dir := range fragmentSearchDirs() {
		for _, importDir := range configImportDirCandidates {
			candidate := filepath.Join(dir, importDir)
			if info, err := os.Stat(candidate); err != nil || !info.IsDir() {
				continue
			}
			files, err := SearchAtmosConfig(candidate)
			if err != nil {
				continue
			}
			for _, file := range files {
				if declares, derr := fileDeclaresTopLevelKey(file, section); derr == nil && declares {
					return file, true
				}
			}
		}
	}
	return "", false
}

// fragmentSearchDirs returns the directories whose atmos.d/.atmos.d fragments are
// in effect: the current working directory first, then the git repository root
// (when different). A CWD fragment is preferred over a git-root one, mirroring
// mergeDefaultImports' precedence.
func fragmentSearchDirs() []string {
	seen := make(map[string]struct{})
	var dirs []string
	add := func(dir string) {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return
		}
		if _, ok := seen[abs]; ok {
			return
		}
		seen[abs] = struct{}{}
		dirs = append(dirs, abs)
	}
	if cwd, err := os.Getwd(); err == nil {
		add(cwd)
	}
	if gitRoot, err := u.ProcessTagGitRoot("!repo-root ."); err == nil && gitRoot != "" {
		add(gitRoot)
	}
	return dirs
}

// fileDeclaresTopLevelKey reports whether the YAML file at path has key at the
// top level of its root mapping. Used to detect which fragment owns a section
// (e.g. "mcp") so edits land in the file that already declares it, rather than
// splitting config across the root atmos.yaml and a fragment.
func fileDeclaresTopLevelKey(path, key string) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var root goyaml.Node
	if err := goyaml.Unmarshal(content, &root); err != nil {
		return false, err
	}
	if len(root.Content) == 0 || root.Content[0].Kind != goyaml.MappingNode {
		return false, nil
	}
	mapping := root.Content[0]
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return true, nil
		}
	}
	return false, nil
}

// resolveOverridePath resolves an explicit override that may point at either a
// file or a directory containing an atmos.yaml.
func resolveOverridePath(override string) (string, error) {
	info, err := os.Stat(override)
	if err != nil {
		// Only a genuinely missing path is "no editable config"; permission or
		// I/O errors must surface with context so users see the real cause.
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", ErrNoEditableConfig, override)
		}
		return "", fmt.Errorf("failed to stat config path %s: %w", override, err)
	}
	if !info.IsDir() {
		return override, nil
	}
	path, ok, probeErr := firstExistingConfig(override)
	if probeErr != nil {
		return "", probeErr
	}
	if ok {
		return path, nil
	}
	return "", fmt.Errorf("%w: no atmos.yaml in %s", ErrNoEditableConfig, override)
}

// firstExistingConfig returns the first existing config file in dir. A missing
// candidate is skipped; any other stat error (permission, I/O) is returned so a
// broken candidate is never silently ignored.
func firstExistingConfig(dir string) (string, bool, error) {
	for _, name := range configFileCandidates {
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", false, fmt.Errorf("failed to stat config file %s: %w", candidate, err)
		}
		if !info.IsDir() {
			return candidate, true, nil
		}
	}
	return "", false, nil
}
