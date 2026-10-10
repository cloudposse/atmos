package hooks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/generic"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/ci/providers/github/ghtest"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
)

// ciHookProbe is a script engine that reports through the CI reporter its host supplied.
type ciHookProbe struct {
	supplied bool
	receipt  ci.Receipt
}

//nolint:gocritic // The signature is fixed by script.Engine.
func (p *ciHookProbe) Execute(_ context.Context, spec script.Spec) (script.Result, error) {
	if spec.CI == nil {
		return script.Result{}, nil
	}
	p.supplied = true
	rc, err := spec.CI.Summary("hook probe\n")
	p.receipt = rc
	return script.Result{}, err
}

// TestStepHookScriptGetsConfiguredCIReporter proves a `kind: step` hook running a script builds
// the reporter from the hook's Atmos configuration, so the script's CI gates reflect ci.enabled
// rather than the nil-config fallback that reads every switch as off.
func TestStepHookScriptGetsConfiguredCIReporter(t *testing.T) {
	server := ghtest.NewServer(t)
	env := ghtest.SetEnv(t, server)
	t.Chdir(t.TempDir())
	ghtest.RegisterProvider(t, github.NewProvider())
	ci.Register(generic.NewProvider())

	probe := &ciHookProbe{}
	const interpreter = "hook-ci-probe"
	script.Register(interpreter, probe)

	kind, ok := GetKind(stepKindName)
	require.True(t, ok)
	hook := &Hook{
		Kind: stepKindName, Type: schema.TaskTypeScript,
		With: map[string]any{"name": "hook", "interpreter": interpreter, "script": "ignored"},
	}
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir(), CI: schema.CIConfig{Enabled: true}}

	_, err := kind.Engine.Run(&ExecContext{
		Hook: hook, Kind: kind, HookName: "hook", AtmosConfig: atmosConfig, Info: &schema.ConfigAndStacksInfo{},
	})
	require.NoError(t, err)

	require.True(t, probe.supplied, "the hook must supply a CI reporter to the script")
	assert.Equal(t, github.ProviderName, probe.receipt.Provider)
	assert.Empty(t, probe.receipt.Gate)
	assert.False(t, probe.receipt.Local)
	assert.Contains(t, ghtest.ReadFile(t, env.Summary), "hook probe")
}
