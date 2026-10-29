package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	"github.com/cloudposse/atmos/pkg/plugin/conformance"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
)

// verifyRegisteredPluginChecks exercises extension hook composition through the
// public runner against the executable created by the CLI acceptance test.
func verifyRegisteredPluginChecks(t *testing.T, directory string) {
	t.Helper()
	defaults, err := bootstrap.Descriptors()
	require.NoError(t, err)
	registry := extension.New()
	calls := 0
	for _, capability := range []string{"auth.provider", "auth.identity", "command"} {
		descriptor, found := defaults.Get(capability)
		require.True(t, found)
		if capability == "command" {
			descriptor.Test = func(ctx context.Context, binding extension.Binding) error {
				require.NoError(t, ctx.Err())
				require.Equal(t, directory, binding.Directory)
				require.Equal(t, "about", binding.Name)
				calls++
				return nil
			}
		}
		require.NoError(t, registry.Register(descriptor))
	}
	report, err := conformance.Run(t.Context(), directory, conformance.Options{
		Registry: registry, Run: "extension/command/about",
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, &conformance.Report{Mode: "mock", Results: []conformance.Result{
		{Name: "manifest-handler-agreement", Passed: true},
		{Name: "extension/command/about", Passed: true},
	}}, report)
}
