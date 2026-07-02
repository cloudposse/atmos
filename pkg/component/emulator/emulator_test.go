package emulator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestProvider_TypeAndGroup(t *testing.T) {
	p := &EmulatorComponentProvider{}
	assert.Equal(t, cfg.EmulatorComponentType, p.GetType())
	assert.Equal(t, "Emulators", p.GetGroup())
	assert.Equal(t, []string{"up", "down", "reset", "ps", "list", "logs", "exec"}, p.GetAvailableCommands())
}

func TestProvider_GetBasePath_Default(t *testing.T) {
	p := &EmulatorComponentProvider{}
	assert.Equal(t, "components/emulator", p.GetBasePath(nil))
}

func TestProvider_ListComponents(t *testing.T) {
	p := &EmulatorComponentProvider{}
	stackConfig := map[string]any{
		"components": map[string]any{
			"emulator": map[string]any{
				"gcp": map[string]any{"driver": "floci/gcp"},
				"aws": map[string]any{"driver": "floci/aws"},
			},
		},
	}
	names, err := p.ListComponents(context.Background(), "dev", stackConfig)
	require.NoError(t, err)
	assert.Equal(t, []string{"aws", "gcp"}, names, "sorted")
}

func TestProvider_ListComponents_NoEmulators(t *testing.T) {
	p := &EmulatorComponentProvider{}
	names, err := p.ListComponents(context.Background(), "dev", map[string]any{"components": map[string]any{}})
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestProvider_ValidateComponent(t *testing.T) {
	p := &EmulatorComponentProvider{}

	t.Run("requires a driver", func(t *testing.T) {
		err := p.ValidateComponent(map[string]any{"cloud": "aws"})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrComponentValidationFailed)
	})

	t.Run("rejects whitespace driver", func(t *testing.T) {
		err := p.ValidateComponent(map[string]any{"driver": "   "})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrComponentValidationFailed)
	})

	t.Run("valid with driver", func(t *testing.T) {
		require.NoError(t, p.ValidateComponent(map[string]any{"driver": "floci/aws"}))
	})

	t.Run("abstract base is skipped", func(t *testing.T) {
		section := map[string]any{"metadata": map[string]any{"type": "abstract"}}
		require.NoError(t, p.ValidateComponent(section))
	})

	t.Run("nil config is allowed", func(t *testing.T) {
		require.NoError(t, p.ValidateComponent(nil))
	})
}

func TestProvider_Execute_UnknownSubcommand(t *testing.T) {
	p := &EmulatorComponentProvider{}
	err := p.Execute(&component.ExecutionContext{SubCommand: "bogus"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrComponentExecutionFailed)
}

func TestEnvListToMap(t *testing.T) {
	out := envListToMap([]string{"A=1", "B=two=2", "noeq", "=skip"})
	assert.Equal(t, map[string]string{"A": "1", "B": "two=2"}, out)
}
