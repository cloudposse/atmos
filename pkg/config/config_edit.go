package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// EffectiveConfigFilesAscending returns the config files that participate in the
// merged configuration, in ascending precedence order (a later file overrides an
// earlier one). Config-editing commands that must edit the file whose value is
// actually effective for a key - rather than guessing by top-level section
// presence - select the last (highest-precedence) candidate that declares the key
// (cloudposse/atmos#3269).
//
// The order mirrors how Atmos loads config:
//
//  1. git-repository-root `atmos.d/` and `.atmos.d/` fragments (lowest precedence);
//  2. the current working directory's `atmos.d/` and `.atmos.d/` fragments, but
//     only when the CWD has its own root atmos.yaml - otherwise the loader uses
//     the git root and the CWD fragments are not merged;
//  3. the root atmos.yaml / .atmos.yaml itself (highest precedence - Atmos
//     reapplies it after its imports, so an explicit root value overrides a
//     fragment).
//
// Within a fragment directory, files follow SearchAtmosConfig order (depth, then
// name), matching the loader's later-overrides-earlier merge.
func EffectiveConfigFilesAscending(atmosConfig *schema.AtmosConfiguration) []string {
	defer perf.Track(atmosConfig, "config.EffectiveConfigFilesAscending")()

	var cwd string
	cwdHasConfig := false
	if wd, err := os.Getwd(); err == nil {
		cwd = wd
		if _, ok, probeErr := firstExistingConfig(wd); probeErr == nil && ok {
			cwdHasConfig = true
		}
	}

	var files []string
	for _, dir := range fragmentDirsAscending(cwd, cwdHasConfig) {
		files = append(files, fragmentFiles(dir)...)
	}
	if root, err := ResolveEditableConfigFile(atmosConfig, ""); err == nil {
		files = append(files, root)
	}
	return files
}

// fragmentDirsAscending returns the directories whose atmos.d/.atmos.d fragments
// are in effect, in ascending precedence: the git repository root first (lowest),
// then the current working directory (higher) when it carries its own root config.
func fragmentDirsAscending(cwd string, cwdHasConfig bool) []string {
	seen := make(map[string]struct{})
	var dirs []string
	add := func(dir string) {
		if dir == "" {
			return
		}
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
	// ProcessTagGitRoot returns "." (not an error) when there is no real git
	// repository, which must not be treated as a root -- otherwise the CWD would be
	// searched here, bypassing the cwdHasConfig gate below. This mirrors the guard
	// in loadAtmosDFromGitRoot.
	if gitRoot, err := u.ProcessTagGitRoot("!repo-root ."); err == nil && gitRoot != "" && gitRoot != "." {
		add(gitRoot)
	}
	if cwdHasConfig {
		add(cwd)
	}
	return dirs
}

// fragmentFiles returns the config fragment files under dir's atmos.d/ and
// .atmos.d/ directories, in SearchAtmosConfig order.
func fragmentFiles(dir string) []string {
	var files []string
	for _, importDir := range configImportDirCandidates {
		base := filepath.Join(dir, importDir)
		if info, err := os.Stat(base); err != nil || !info.IsDir() {
			continue
		}
		found, err := SearchAtmosConfig(base)
		if err != nil {
			continue
		}
		files = append(files, found...)
	}
	return files
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
