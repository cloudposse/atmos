package backend

import (
	"context"
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestBackendInspectionInvalidOptionsBeforeDiscovery(t *testing.T) {
	original := inspectBackendStacks
	t.Cleanup(func() { inspectBackendStacks = original })
	inspectBackendStacks = func(*schema.AtmosConfiguration, string) (map[string]any, error) {
		t.Fatal("discovery must not run")
		return nil, nil
	}
	require.ErrorIs(t, executeListCommandWithValues("dev", "xml"), errUtils.ErrInvalidFlagValue)
	require.ErrorIs(t, executeDescribeCommandWithValues("a", "dev", "xml"), errUtils.ErrInvalidFlagValue)
	require.ErrorIs(t, executeListCommandWithValues("", "json"), errUtils.ErrRequiredFlagNotProvided)
	require.ErrorIs(t, executeDescribeCommandWithValues("", "dev", "json"), errUtils.ErrComponentRequired)
}

func TestBackendInspectionCommandsUseStaticDiscovery(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command *cobra.Command
		args    []string
	}{
		{"list", listCmd, nil},
		{"describe", describeCmd, []string{"a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldContext := tc.command.Context()
			t.Cleanup(func() { tc.command.SetContext(oldContext) })
			tc.command.SetContext(context.Background())
			// No expectation permits authentication: any InitConfigAndAuth call fails the test.
			setupTestWithMocks(t)
			setupViperForTest(t, map[string]any{"stack": "dev", "format": "json", "identity": "must-not-authenticate"})
			previous, previousInspect := atmosConfigPtr, inspectBackendStacks
			t.Cleanup(func() { atmosConfigPtr, inspectBackendStacks = previous, previousInspect })
			atmosConfigPtr = &schema.AtmosConfiguration{BasePath: "selected-config"}
			inspectBackendStacks = func(config *schema.AtmosConfiguration, stack string) (map[string]any, error) {
				require.Same(t, atmosConfigPtr, config)
				require.Equal(t, "selected-config", config.BasePath)
				require.Equal(t, "dev", stack)
				return nil, io.ErrUnexpectedEOF
			}
			require.ErrorIs(t, tc.command.RunE(tc.command, tc.args), io.ErrUnexpectedEOF)
		})
	}
}

func TestBackendInspectionCancelledBeforeDiscovery(t *testing.T) {
	for _, command := range []*cobra.Command{listCmd, describeCmd} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		old := command.Context()
		command.SetContext(ctx)
		require.ErrorIs(t, command.RunE(command, nil), context.Canceled)
		command.SetContext(old)
	}
}

func TestBackendInspectionCompletion(t *testing.T) {
	oldConfig, oldInspect := atmosConfigPtr, inspectBackendStacks
	t.Cleanup(func() { atmosConfigPtr, inspectBackendStacks = oldConfig, oldInspect })
	atmosConfigPtr = &schema.AtmosConfiguration{}
	inspectBackendStacks = func(_ *schema.AtmosConfiguration, stack string) (map[string]any, error) {
		return map[string]any{
			"dev": map[string]any{"components": map[string]any{"terraform": map[string]any{
				"a": map[string]any{}, "base": map[string]any{"metadata": map[string]any{"type": "abstract"}},
			}}},
			"prod": map[string]any{"components": map[string]any{"terraform": map[string]any{"z": map[string]any{}}}},
		}, nil
	}
	names, directive := inspectionStackCompletion(nil, nil, "")
	require.Equal(t, []string{"dev", "prod"}, names)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	names, _ = inspectionStackCompletion(nil, []string{"a"}, "")
	require.Equal(t, []string{"dev"}, names)
	names, _ = inspectionComponentCompletion(describeCmd, nil, "")
	require.Equal(t, []string{"a", "z"}, names)
	names, _ = inspectionComponentCompletion(describeCmd, []string{"a"}, "")
	require.Empty(t, names)
	inspectBackendStacks = func(*schema.AtmosConfiguration, string) (map[string]any, error) { return nil, io.ErrUnexpectedEOF }
	names, _ = inspectionStackCompletion(nil, nil, "")
	require.Empty(t, names)
	names, _ = inspectionComponentCompletion(describeCmd, nil, "")
	require.Empty(t, names)
	atmosConfigPtr = nil
	names, _ = inspectionStackCompletion(nil, nil, "")
	require.Empty(t, names)
	names, _ = inspectionComponentCompletion(describeCmd, nil, "")
	require.Empty(t, names)
}
