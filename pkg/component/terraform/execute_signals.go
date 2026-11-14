package terraform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/execution"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// clearStaleSignals runs once during unit preparation, before read-config hooks
// or a source refresh can produce new signals for this invocation.
func clearStaleSignals(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	if info.DryRun {
		return nil
	}
	moduleDir, err := signalModuleDirectory(atmosConfig, info)
	if err != nil {
		return err
	}
	// A module directory that the source fetch has not created yet holds no stale signals.
	if moduleDir == "" {
		return nil
	}
	path := filepath.Join(moduleDir, execution.SignalsFileName)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: clear stale error signals from %s: %w", errUtils.ErrGeneratorRemoveFailed, path, err)
	}
	return nil
}

// signalModuleDirectory returns the module directory the unit runs in, or "" when it does not exist yet.
func signalModuleDirectory(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error) {
	section := info.ComponentSection
	if source.HasSource(section) {
		target, err := source.ResolveTarget(atmosConfig, cfg.TerraformComponentType, info.FinalComponent, section)
		if err != nil {
			return "", err
		}
		if !target.IsWorkdir {
			return target.Dir, nil
		}
		return existingSubpath(info, target.Dir)
	}
	if workdir.IsWorkdirEnabled(section) {
		root, err := workdir.BuildPath(atmosConfig.BasePath, cfg.TerraformComponentType, info.FinalComponent, info.Stack, section)
		if err != nil {
			return "", err
		}
		return existingSubpath(info, root)
	}
	return ConstructTerraformComponentWorkingDir(atmosConfig, info), nil
}

// existingSubpath resolves the module directory under root, or "" when an explicit source subdirectory
// has not been fetched yet.
func existingSubpath(info *schema.ConfigAndStacksInfo, root string) (string, error) {
	moduleDir, present, err := component.ResolvePendingSubpath(info, root)
	if err != nil || !present {
		return "", err
	}
	return moduleDir, nil
}
