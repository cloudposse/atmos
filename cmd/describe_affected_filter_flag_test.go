package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
)

// TestNewDescribeAffectedSelectorFlagsParser_RegistersFilter verifies the --filter flag type and usage.
func TestNewDescribeAffectedSelectorFlagsParser_RegistersFilter(t *testing.T) {
	cmd := &cobra.Command{Use: "affected"}
	newDescribeAffectedSelectorFlagsParser().RegisterPersistentFlags(cmd)

	filter := cmd.PersistentFlags().Lookup(describeAffectedFilterFlagName)
	require.NotNil(t, filter)
	assert.Equal(t, "string", filter.Value.Type())
	assert.Empty(t, filter.DefValue)
	assert.NotEmpty(t, filter.Usage)
}

// TestDescribeAffectedCmd_RegistersFilterFlag confirms the real command exposes --filter.
func TestDescribeAffectedCmd_RegistersFilterFlag(t *testing.T) {
	_ = NewTestKit(t)

	assert.NotNil(t, describeAffectedCmd.PersistentFlags().Lookup("filter"))
}

// TestResolveDescribeAffectedSelectorFlags_Filter verifies --filter resolution from the command line and
// ATMOS_DESCRIBE_AFFECTED_FILTER, and that only env-sourced values carry the env mark.
func TestResolveDescribeAffectedSelectorFlags_Filter(t *testing.T) {
	const expr = ".settings.github.actions_enabled // false"

	tests := []struct {
		name       string
		cliArgs    []string
		env        map[string]string
		wantFilter string
		wantSet    bool
		wantEnv    string
	}{
		{name: "nothing set leaves the flag untouched"},
		{
			name:       "env var populates the unset flag",
			env:        map[string]string{"ATMOS_DESCRIBE_AFFECTED_FILTER": expr},
			wantFilter: expr,
			wantSet:    true,
			wantEnv:    "ATMOS_DESCRIBE_AFFECTED_FILTER",
		},
		{
			name:       "CLI flag wins over the env var and is not marked as env-sourced",
			cliArgs:    []string{"--filter=.component == \"vpc\""},
			env:        map[string]string{"ATMOS_DESCRIBE_AFFECTED_FILTER": expr},
			wantFilter: `.component == "vpc"`,
			wantSet:    true,
		},
		{
			name:       "an explicitly empty CLI flag overrides the env var",
			cliArgs:    []string{"--filter="},
			env:        map[string]string{"ATMOS_DESCRIBE_AFFECTED_FILTER": expr},
			wantFilter: "",
			wantSet:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			cmd, v, p := newSelectorFlagsTestCmd(t, tt.cliArgs...)

			require.NoError(t, resolveDescribeAffectedSelectorFlags(cmd, v, p))

			got, err := cmd.Flags().GetString("filter")
			require.NoError(t, err)
			assert.Equal(t, tt.wantFilter, got)
			assert.Equal(t, tt.wantSet, cmd.Flags().Changed("filter"))

			envVar, ok := flags.FlagValueFromEnv(cmd.Flags().Lookup("filter"))
			assert.Equal(t, tt.wantEnv != "", ok)
			assert.Equal(t, tt.wantEnv, envVar)
		})
	}
}

// TestResolveDescribeAffectedSelectorFlags_FilterClearsStaleEnvMark guards the long-lived command object:
// a mark from an earlier env-sourced run must not survive a later run that sets --filter explicitly.
func TestResolveDescribeAffectedSelectorFlags_FilterClearsStaleEnvMark(t *testing.T) {
	cmd, v, p := newSelectorFlagsTestCmd(t, "--filter=.component")
	flags.MarkFlagValueFromEnv(cmd.Flags().Lookup("filter"), "ATMOS_DESCRIBE_AFFECTED_FILTER")

	require.NoError(t, resolveDescribeAffectedSelectorFlags(cmd, v, p))

	_, ok := flags.FlagValueFromEnv(cmd.Flags().Lookup("filter"))
	assert.False(t, ok)
}
