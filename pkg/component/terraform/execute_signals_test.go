package terraform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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
