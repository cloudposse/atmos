package backend

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestBackendMutations_HonorInheritedDryRun(t *testing.T) {
	for _, command := range backendSubcommandCases()[:3] {
		t.Run(command.name, func(t *testing.T) {
			for _, tc := range []struct {
				name   string
				flag   string
				config bool
				want   bool
			}{
				{name: "inherited flag", flag: "true", want: true},
				{name: "configuration", config: true, want: true},
				{name: "explicit false", flag: "false", config: true},
				{name: "normal execution"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					mockConfig := setupTestWithMocks(t)
					setupViperForTest(t, map[string]any{"stack": "dev", "dry-run": tc.config})

					// Copy the command's flags into a fresh tree so Cobra's cached
					// inherited flags cannot leak between runs of this test.
					parent := &cobra.Command{Use: "cloudformation"}
					parent.PersistentFlags().Bool("dry-run", false, "Dry run")
					cmd := &cobra.Command{Use: command.cmd.Use, RunE: command.cmd.RunE}
					cmd.Flags().AddFlagSet(command.cmd.Flags())
					parent.AddCommand(cmd)
					if tc.flag != "" {
						require.NoError(t, parent.PersistentFlags().Set("dry-run", tc.flag))
					}

					stack := command.cmd.Flags().Lookup("stack")
					oldValue, oldChanged := stack.Value.String(), stack.Changed
					require.NoError(t, command.cmd.Flags().Set("stack", "dev"))
					t.Cleanup(func() {
						require.NoError(t, stack.Value.Set(oldValue))
						stack.Changed = oldChanged
					})

					stop := errors.New("normal execution reached config initialization")
					if !tc.want {
						mockConfig.EXPECT().InitConfigAndAuth("vpc", "dev", "").Return(nil, nil, stop)
					}
					err := cmd.RunE(cmd, []string{"vpc"})
					if tc.want {
						// Strict mocks reject authentication, confirmation checks,
						// component resolution and every backend mutation.
						require.NoError(t, err)
					} else {
						require.ErrorIs(t, err, stop)
					}
				})
			}
		})
	}
}

func TestBackendDryRun_StillRequiresStack(t *testing.T) {
	setupTestWithMocks(t)
	require.ErrorIs(t, executeCreateOrUpdate(t.Context(), createOrUpdateArgs{DryRun: true}), errUtils.ErrRequiredFlagNotProvided)
	require.ErrorIs(t, executeDelete(t.Context(), deleteRequest{DryRun: true}), errUtils.ErrRequiredFlagNotProvided)
}
