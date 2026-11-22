package terraform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/execution"

	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestClearStaleSignalsUsesProvisionedModuleDirectory(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	section := map[string]any{
		"component": "example", "atmos_stack": "dev",
		"source":    map[string]any{"uri": "git::https://example.test/repo.git"},
		"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
	}
	info := &schema.ConfigAndStacksInfo{ComponentSection: section, FinalComponent: "example", Stack: "dev", BaseComponentPath: "module"}
	target, err := source.ResolveTarget(atmosConfig, "terraform", "example", section)
	require.NoError(t, err)
	moduleDir := filepath.Join(target.Dir, "module")
	require.NoError(t, os.MkdirAll(moduleDir, 0o755))
	stale := filepath.Join(moduleDir, "error-signals.json")
	require.NoError(t, os.WriteFile(stale, []byte(`{"old":true}`), 0o644))
	require.NoError(t, clearStaleSignals(atmosConfig, info))
	require.NoFileExists(t, stale)

	info.DryRun = true
	require.NoError(t, os.WriteFile(stale, []byte(`{"old":true}`), 0o644))
	require.NoError(t, clearStaleSignals(atmosConfig, info))
	require.FileExists(t, stale, "dry runs leave the module untouched")
}

// TestClearStaleSignalsWrapsRemovalFailures checks that a signals file that cannot be removed fails
// with the static Atmos error and keeps the operating-system error as its cause.
func TestClearStaleSignalsWrapsRemovalFailures(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	section := map[string]any{
		"component": "example", "atmos_stack": "dev",
		"source":    map[string]any{"uri": "git::https://example.test/repo.git"},
		"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
	}
	info := &schema.ConfigAndStacksInfo{ComponentSection: section, FinalComponent: "example", Stack: "dev", BaseComponentPath: "module"}
	target, err := source.ResolveTarget(atmosConfig, "terraform", "example", section)
	require.NoError(t, err)
	// A non-empty directory in place of the signals file cannot be removed on any platform.
	blocked := filepath.Join(target.Dir, "module", "error-signals.json")
	require.NoError(t, os.MkdirAll(filepath.Join(blocked, "child"), 0o755))

	err = clearStaleSignals(atmosConfig, info)
	require.ErrorIs(t, err, errUtils.ErrGeneratorRemoveFailed)
	var pathErr *os.PathError
	require.True(t, errors.As(err, &pathErr), "the operating-system error is preserved: %v", err)
	require.DirExists(t, blocked)
}

// A source subdirectory that the fetch has not created yet holds no stale signals, so preparation proceeds
// to the fetch instead of failing on the missing directory.
func TestClearStaleSignalsToleratesAnUnfetchedSourceSubdirectory(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	section := map[string]any{
		"component": "example", "atmos_stack": "dev",
		"source":    map[string]any{"uri": "git::https://example.test/repo.git"},
		"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
		"metadata":  map[string]any{"source_subdir": "modules/app"},
	}
	info := &schema.ConfigAndStacksInfo{ComponentSection: section, FinalComponent: "example", Stack: "dev"}
	require.NoError(t, clearStaleSignals(atmosConfig, info))

	target, err := source.ResolveTarget(atmosConfig, "terraform", "example", section)
	require.NoError(t, err)
	moduleDir := filepath.Join(target.Dir, "modules", "app")
	require.NoError(t, os.MkdirAll(moduleDir, 0o755))
	stale := filepath.Join(moduleDir, "error-signals.json")
	require.NoError(t, os.WriteFile(stale, []byte(`{"old":true}`), 0o644))
	require.NoError(t, clearStaleSignals(atmosConfig, info))
	require.NoFileExists(t, stale, "a fetched module directory is still cleared")

	section["metadata"] = map[string]any{"source_subdir": "../escape"}
	require.ErrorIs(t, clearStaleSignals(atmosConfig, info), errUtils.ErrWorkdirProvision, "an invalid subdirectory still fails")
}

func TestIgnoredPreparationRefreshesRetainedSignals(t *testing.T) {
	for _, signals := range []map[string]any{nil, {"current": true}} {
		t.Run(fmt.Sprint(signals), func(t *testing.T) {
			root := t.TempDir()
			cfg := &schema.AtmosConfiguration{TerraformDirAbsolutePath: root}
			info := &schema.ConfigAndStacksInfo{FinalComponent: "app"}
			dir := ConstructTerraformComponentWorkingDir(cfg, info)
			require.NoError(t, os.MkdirAll(dir, 0o755))
			path := filepath.Join(dir, execution.SignalsFileName)
			require.NoError(t, os.WriteFile(path, []byte(`{"stale":true}`), 0o600))
			prepared := &schema.PreparedComponentConfiguration{IgnoredRun: &schema.ComponentIgnoredRun{Policy: "registry"}, Execution: &schema.ComponentExecution{Errors: &schema.ComponentErrors{Ignore: []schema.ComponentIgnorePolicy{{Name: "registry", Signals: signals}}}}}
			require.NoError(t, refreshIgnoredSignals(cfg, info, prepared))
			if signals == nil {
				require.NoFileExists(t, path)
			} else {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				require.JSONEq(t, `{"current":true}`, string(data))
			}
		})
	}
}

// An ordinary Terraform component has no provider preparation, so a file named like the signals
// file in its directory belongs to the user.
func TestOrdinaryComponentKeepsAFileNamedLikeTheSignalsFile(t *testing.T) {
	root := t.TempDir()
	cfg := &schema.AtmosConfiguration{TerraformDirAbsolutePath: root}
	info := &schema.ConfigAndStacksInfo{FinalComponent: "app"}
	dir := ConstructTerraformComponentWorkingDir(cfg, info)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	owned := filepath.Join(dir, execution.SignalsFileName)
	require.NoError(t, os.WriteFile(owned, []byte(`{"owner":"user"}`), 0o600))

	require.NoError(t, (&Executor{}).beginPreparedConfiguration(t.Context(), cfg, info, nil))
	require.FileExists(t, owned)

	// A prepared unit still starts from a clean signals file.
	prepared := &schema.PreparedComponentConfiguration{}
	require.NoError(t, (&Executor{}).beginPreparedConfiguration(t.Context(), cfg, info, prepared))
	require.NoFileExists(t, owned)
}
