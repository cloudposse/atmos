package helm

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveHelmBinary(t *testing.T) {
	origInit := initHelmCliConfig
	origDeps := dependenciesForHelmComponent
	t.Cleanup(func() {
		initHelmCliConfig = origInit
		dependenciesForHelmComponent = origDeps
	})

	t.Run("resolves the helm binary", func(t *testing.T) {
		initHelmCliConfig = func(_ schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
			assert.False(t, processStacks)
			return schema.AtmosConfiguration{}, nil
		}
		dependenciesForHelmComponent = func(_ *schema.AtmosConfiguration, componentType string, _ map[string]any, _ map[string]any) (*dependencies.ToolchainEnvironment, error) {
			assert.Equal(t, cfg.HelmComponentType, componentType)
			return &dependencies.ToolchainEnvironment{}, nil
		}
		got, err := resolveHelmBinary(&cobra.Command{Use: "list"})
		require.NoError(t, err)
		// Resolve returns the bare name ("helm") when nothing is installed, or a
		// toolchain/PATH-resolved path (e.g. ".../helm/v3.19.2/helm[.exe]") when it
		// is — assert the binary name rather than the environment-dependent path.
		base := strings.TrimSuffix(filepath.Base(got), ".exe")
		assert.Equal(t, "helm", base)
	})

	t.Run("config init error", func(t *testing.T) {
		sentinel := errors.New("config failed")
		initHelmCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, sentinel
		}
		_, err := resolveHelmBinary(&cobra.Command{Use: "list"})
		require.ErrorIs(t, err, sentinel)
	})

	t.Run("dependency resolution error", func(t *testing.T) {
		sentinel := errors.New("deps failed")
		initHelmCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		}
		dependenciesForHelmComponent = func(*schema.AtmosConfiguration, string, map[string]any, map[string]any) (*dependencies.ToolchainEnvironment, error) {
			return nil, sentinel
		}
		_, err := resolveHelmBinary(&cobra.Command{Use: "list"})
		require.ErrorIs(t, err, sentinel)
	})
}
