package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/auth"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPackageTimeoutReachesTheActivation(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, "bin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("static"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(strings.ReplaceAll(automaticManifest, "PLATFORM", install.CurrentPlatform())), 0o600))
	constructors := newConstructors(&adapter.Services{})
	registry, err := descriptors(constructors)
	require.NoError(t, err)
	state := &config.Selections{Packages: map[string]config.Selection{"fixture": {Path: directory, Executable: "bin/plugin"}}}

	require.NoError(t, activatePackages(t.Context(), state, registry, map[string]schema.PluginPackage{"fixture": {Timeout: "5m"}}))
	require.Len(t, constructors.Commands, 1)
	for _, activation := range constructors.Commands {
		assert.Equal(t, 5*time.Minute, activation.Timeout)
	}

	// Negative: no timeout configured means no deadline.
	constructors = newConstructors(&adapter.Services{})
	registry, err = descriptors(constructors)
	require.NoError(t, err)
	require.NoError(t, activatePackages(t.Context(), state, registry, map[string]schema.PluginPackage{"fixture": {}}))
	for _, activation := range constructors.Commands {
		assert.Zero(t, activation.Timeout)
	}

	// An invalid duration fails with a configuration error rather than running unbounded.
	constructors = newConstructors(&adapter.Services{})
	registry, err = descriptors(constructors)
	require.NoError(t, err)
	err = activatePackages(t.Context(), state, registry, map[string]schema.PluginPackage{"fixture": {Timeout: "soon"}})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}
