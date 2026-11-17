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

// refreshIgnoredSignals replaces stale signals in a retained module after preparation is ignored.
func refreshIgnoredSignals(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) error {
	if info.DryRun {
		return nil
	}
	if err := clearStaleSignals(atmosConfig, info); err != nil {
		return err
	}
	dir, err := signalModuleDirectory(atmosConfig, info)
	if err != nil || dir == "" {
		return err
	}
	if present, err := signalDirectoryExists(dir); err != nil || !present {
		return err
	}
	writeIgnoredPolicySignals(info, prepared, dir)
	return nil
}

// writeIgnoredPolicySignals preserves the signals of the policy selected during preparation.
func writeIgnoredPolicySignals(info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration, dir string) {
	if prepared.Execution == nil || prepared.Execution.Errors == nil {
		return
	}
	for _, policy := range prepared.Execution.Errors.Ignore {
		if policy.Name == prepared.IgnoredRun.Policy {
			execution.NewRecoverer(nil, execution.WithReporter(&recoveryReporter{info: info}), execution.WithSignalsFile(filepath.Join(dir, execution.SignalsFileName))).WriteSignals(&policy)
			return
		}
	}
}

// signalDirectoryExists avoids creating a module directory for an ignored preparation.
func signalDirectoryExists(dir string) (bool, error) {
	stat, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return stat.IsDir(), nil
}
