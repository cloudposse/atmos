package helm

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	t.Run("falls back to PATH name", func(t *testing.T) {
		initHelmCliConfig = func(_ schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
			assert.False(t, processStacks)
			return schema.AtmosConfiguration{}, nil
		}
		dependenciesForHelmComponent = func(*schema.AtmosConfiguration, string, map[string]any, map[string]any) (*dependencies.ToolchainEnvironment, error) {
			return &dependencies.ToolchainEnvironment{}, nil
		}
		got, err := resolveHelmBinary(&cobra.Command{Use: "list"})
		require.NoError(t, err)
		assert.Equal(t, "helm", got)
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
