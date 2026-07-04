package kubernetes

import (
	"errors"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKubernetesCIModeEnabled(t *testing.T) {
	t.Setenv("ATMOS_CI", "")
	t.Setenv("CI", "")

	assert.True(t, kubernetesCIModeEnabled(map[string]any{"ci": true}))
	assert.False(t, kubernetesCIModeEnabled(map[string]any{"ci": false}))
	assert.False(t, kubernetesCIModeEnabled(map[string]any{}))
	assert.False(t, kubernetesCIModeEnabled(nil))

	t.Setenv("ATMOS_CI", "1")
	assert.True(t, kubernetesCIModeEnabled(map[string]any{}))

	t.Setenv("ATMOS_CI", "false")
	t.Setenv("CI", "yes")
	assert.True(t, kubernetesCIModeEnabled(map[string]any{}))

	t.Setenv("CI", "0")
	assert.False(t, kubernetesCIModeEnabled(map[string]any{}))
}

func TestRunKubernetesCIHookBuildsAggregateForNilResult(t *testing.T) {
	original := runKubernetesCIHooks
	t.Cleanup(func() { runKubernetesCIHooks = original })

	commandErr := errors.New("kubectl failed")
	var captured *hooks.RunCIHooksOptions
	runKubernetesCIHooks = func(opts *hooks.RunCIHooksOptions) error {
		captured = opts
		return nil
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "api",
		Stack:            "dev",
		SubCommand:       "apply",
	}

	runKubernetesCIHook(
		hooks.AfterKubernetesApply,
		map[string]any{"ci": true},
		&schema.AtmosConfiguration{},
		info,
		nil,
		commandErr,
	)

	require.NotNil(t, captured)
	assert.Equal(t, hooks.AfterKubernetesApply, captured.Event)
	assert.True(t, captured.ForceCIMode)
	assert.Equal(t, commandErr, captured.CommandError)
	assert.Equal(t, errUtils.GetExitCode(commandErr), captured.ExitCode)

	result, ok := captured.Aggregate.(*schema.KubernetesCIResult)
	require.True(t, ok)
	assert.Equal(t, "api", result.Component)
	assert.Equal(t, "dev", result.Stack)
	assert.Equal(t, "apply", result.Command)
	assert.Equal(t, commandErr.Error(), result.Error)
	assert.Equal(t, errUtils.GetExitCode(commandErr), result.ExitCode)
}

func TestRunKubernetesCIHookSwallowsHookError(t *testing.T) {
	original := runKubernetesCIHooks
	t.Cleanup(func() { runKubernetesCIHooks = original })

	called := false
	runKubernetesCIHooks = func(*hooks.RunCIHooksOptions) error {
		called = true
		return errors.New("hook failed")
	}

	assert.NotPanics(t, func() {
		runKubernetesCIHook(
			hooks.AfterKubernetesDiff,
			map[string]any{},
			&schema.AtmosConfiguration{},
			&schema.ConfigAndStacksInfo{},
			&schema.KubernetesCIResult{},
			nil,
		)
	})
	assert.True(t, called)
}
