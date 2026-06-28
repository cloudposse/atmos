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
		if path, ok := firstExistingConfig(cwd); ok {
			return path, nil
		}
	}

	if gitRoot, gitErr := u.ProcessTagGitRoot("!repo-root ."); gitErr == nil && gitRoot != "" {
		if path, ok := firstExistingConfig(gitRoot); ok {
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
		return "", fmt.Errorf("%w: %s", ErrNoEditableConfig, override)
	}
	if !info.IsDir() {
		return override, nil
	}
	if path, ok := firstExistingConfig(override); ok {
		return path, nil
	}
	return "", fmt.Errorf("%w: no atmos.yaml in %s", ErrNoEditableConfig, override)
}

// firstExistingConfig returns the first existing config file in dir.
func firstExistingConfig(dir string) (string, bool) {
	for _, name := range configFileCandidates {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}
