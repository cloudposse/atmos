package emulator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProvider_GetBasePath_FromConfig(t *testing.T) {
	p := &EmulatorComponentProvider{}
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Components.Plugins = map[string]any{
		cfg.EmulatorComponentType: map[string]any{"base_path": "custom/emulators"},
	}
	assert.Equal(t, "custom/emulators", p.GetBasePath(atmosConfig))
}

func TestProvider_GetBasePath_EmptyConfigFallsBackToDefault(t *testing.T) {
	p := &EmulatorComponentProvider{}
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Components.Plugins = map[string]any{
		cfg.EmulatorComponentType: map[string]any{"base_path": ""},
	}
	assert.Equal(t, defaultBasePath, p.GetBasePath(atmosConfig))
}

func TestProvider_GetBasePath_NoConfigFallsBackToDefault(t *testing.T) {
	p := &EmulatorComponentProvider{}
	assert.Equal(t, defaultBasePath, p.GetBasePath(&schema.AtmosConfiguration{}))
}

func TestProvider_GenerateArtifacts_NoOp(t *testing.T) {
	p := &EmulatorComponentProvider{}
	require.NoError(t, p.GenerateArtifacts(&component.ExecutionContext{}))
}

func TestProvider_Execute_DispatchesToVerb(t *testing.T) {
	mgr := &fakeManager{}
	stubPrepare(t, validSection(), nil, mgr)

	execCtx := &component.ExecutionContext{
		SubCommand:          "down",
		ConfigAndStacksInfo: schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "aws"},
	}
	require.NoError(t, newProvider().Execute(execCtx))
	assert.Equal(t, 1, mgr.downCalls)
}

func TestProvider_Execute_ExecForwardsCommand(t *testing.T) {
	mgr := &fakeManager{}
	stubPrepare(t, validSection(), nil, mgr)

	execCtx := &component.ExecutionContext{
		SubCommand:          "exec",
		Args:                []string{"sh", "-c", "echo hi"},
		ConfigAndStacksInfo: schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "aws"},
	}
	require.NoError(t, newProvider().Execute(execCtx))
	assert.Equal(t, []string{"sh", "-c", "echo hi"}, mgr.gotCommand)
}

// newProvider constructs a provider for the dispatch tests.
func newProvider() *EmulatorComponentProvider { return &EmulatorComponentProvider{} }
