package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ErrNoEditableConfig is returned when an editable atmos.yaml file cannot be located.
var ErrNoEditableConfig = errors.New("could not locate an editable atmos.yaml; pass --config to target a specific file")

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
