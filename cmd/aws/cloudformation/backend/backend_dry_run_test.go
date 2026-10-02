package backend

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
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
					if tc.want {
						// A dry run resolves the component from static configuration only.
						mockConfig.EXPECT().DescribeComponentStatic("vpc", "dev").Return(singleS3TargetComponentConfig(), nil)
					} else {
						mockConfig.EXPECT().InitConfigAndAuth("vpc", "dev", "").Return(nil, nil, stop)
					}
					err := cmd.RunE(cmd, []string{"vpc"})
					if tc.want {
						// Strict mocks reject authentication, confirmation checks,
						// authenticated component resolution and every backend mutation.
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
	require.ErrorIs(t, executeCreateOrUpdate(t.Context(), createOrUpdateArgs{Component: "vpc", DryRun: true}), errUtils.ErrRequiredFlagNotProvided)
	require.ErrorIs(t, executeDelete(t.Context(), deleteRequest{Component: "vpc", DryRun: true}), errUtils.ErrRequiredFlagNotProvided)
}

// captureStderr returns what fn writes to the UI channel (stderr).
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	fn()
	require.NoError(t, w.Close())
	os.Stderr = oldStderr

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return strings.Join(strings.Fields(ansi.Strip(string(out))), " ")
}

// A dry run names what it would do, using only static configuration.
func TestBackendDryRun_DescribesWhatWouldHappen(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "create",
			run: func() error {
				return executeCreateOrUpdate(t.Context(), createOrUpdateArgs{Verb: verbCreate, Component: "vpc", Stack: "dev", DryRun: true})
			},
			want: "Dry run: backend create vpc in dev: would create bucket my-bucket in region us-east-1; no AWS calls made",
		},
		{
			name: "update",
			run: func() error {
				return executeCreateOrUpdate(t.Context(), createOrUpdateArgs{Verb: verbUpdate, Component: "vpc", Stack: "dev", DryRun: true})
			},
			want: "Dry run: backend update vpc in dev: would update (creating it if missing) bucket my-bucket in region us-east-1; no AWS calls made",
		},
		{
			name: "delete",
			run: func() error {
				return executeDelete(t.Context(), deleteRequest{Component: "vpc", Stack: "dev", DryRun: true})
			},
			want: "Dry run: backend delete vpc in dev: would delete bucket my-bucket in region us-east-1; no AWS calls made",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := setupTestWithMocks(t)
			mockConfig.EXPECT().DescribeComponentStatic("vpc", "dev").Return(singleS3TargetComponentConfig(), nil)

			var err error
			out := captureStderr(t, func() { err = tt.run() })

			require.NoError(t, err)
			assert.Contains(t, out, tt.want)
		})
	}
}

// A dry run for a component or stack that does not exist fails exactly as the real run would,
// instead of exiting 0 silently. Strict mocks forbid any authentication or mutation.
func TestBackendDryRun_FailsForMissingComponentOrTarget(t *testing.T) {
	missing := errors.New("could not find the component")
	tests := []struct {
		name      string
		config    map[string]any
		describe  error
		wantErrIs error
	}{
		{name: "component or stack does not exist", describe: missing, wantErrIs: missing},
		{name: "component has no aws/s3 target", config: map[string]any{}, wantErrIs: errUtils.ErrInvalidAwsCloudFormationSettings},
	}
	for _, tt := range tests {
		for _, verb := range []string{verbCreate, verbDelete} {
			t.Run(tt.name+"/"+verb, func(t *testing.T) {
				mockConfig := setupTestWithMocks(t)
				mockConfig.EXPECT().DescribeComponentStatic("nonexistent-comp", "no-such-stack").Return(tt.config, tt.describe)

				var err error
				if verb == verbCreate {
					err = executeCreateOrUpdate(t.Context(), createOrUpdateArgs{Verb: verb, Component: "nonexistent-comp", Stack: "no-such-stack", DryRun: true})
				} else {
					err = executeDelete(t.Context(), deleteRequest{Component: "nonexistent-comp", Stack: "no-such-stack", DryRun: true})
				}
				require.ErrorIs(t, err, tt.wantErrIs)
			})
		}
	}
}
