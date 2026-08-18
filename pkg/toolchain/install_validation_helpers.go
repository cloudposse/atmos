package toolchain

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
)

// versionLookupResult holds the result of looking up a tool version.
type versionLookupResult struct {
	tool    string
	version string
}

// resolveVersionFromToolVersions resolves a version from .tool-versions file or uses "latest".
func resolveVersionFromToolVersions(tool, toolSpec string) (versionLookupResult, error) {
	toolVersions, err := LoadToolVersions(DefaultToolVersionsFilePath)
	if err != nil {
		return versionLookupResult{}, errUtils.Build(errUtils.ErrInvalidToolSpec).
			WithExplanationf("Invalid tool specification: `%s`", toolSpec).
			WithHint("Use format: `owner/repo@version` (e.g., `hashicorp/terraform@1.5.0`)").
			WithHint("Or use alias: `terraform@1.5.0` (requires `.tool-versions` or registry alias)").
			WithHint("File `.tool-versions` could not be loaded").
			WithContext("tool_spec", toolSpec).
			WithContext("tool_versions_file", DefaultToolVersionsFilePath).
			WithContext("error", err.Error()).
			WithExitCode(2).
			Err()
	}

	installer := NewInstaller()
	result := LookupToolVersionOrLatest(tool, toolVersions, installer.GetResolver())
	if !result.Found && !result.UsedLatest {
		return versionLookupResult{}, errUtils.Build(errUtils.ErrInvalidToolSpec).
			WithExplanationf("Invalid tool specification: `%s`", toolSpec).
			WithHint("Use format: `owner/repo@version` (e.g., `hashicorp/terraform@1.5.0`)").
			WithHint("Or add tool to `.tool-versions` file").
			WithContext("tool_spec", toolSpec).
			WithExitCode(2).
			Err()
	}

	return versionLookupResult{
		tool:    result.ResolvedKey,
		version: result.Version,
	}, nil
}

// validateToolAndVersion validates that both tool and version are non-empty and that
// version is a format ParseVersionSpec recognizes, before any network call is made.
func validateToolAndVersion(tool, version, toolSpec string) error {
	if tool == "" || version == "" {
		return errUtils.Build(errUtils.ErrInvalidToolSpec).
			WithExplanationf("Invalid tool specification: `%s`", toolSpec).
			WithHint("Use format: `owner/repo@version` (e.g., `hashicorp/terraform@1.5.0`)").
			WithHint("Or use alias: `terraform@1.5.0`").
			WithContext("tool_spec", toolSpec).
			WithExitCode(2).
			Err()
	}
	return ValidateVersionSpec(version)
}

// updateToolVersionsFile updates the .tool-versions file with the installed tool.
// Uses GetToolVersionsFilePath(), not the DefaultToolVersionsFilePath constant, so
// a configured --tool-versions / toolchain.versions_file path is respected instead
// of always writing to the relative ".tool-versions" in the current directory.
func updateToolVersionsFile(tool, version string, setAsDefault bool) error {
	filePath := GetToolVersionsFilePath()
	if setAsDefault {
		if err := AddToolToVersionsAsDefault(filePath, tool, version); err != nil {
			return fmt.Errorf("failed to update .tool-versions: %w", err)
		}
	} else {
		if err := AddToolToVersions(filePath, tool, version); err != nil {
			return fmt.Errorf("failed to update .tool-versions: %w", err)
		}
	}
	return nil
}
