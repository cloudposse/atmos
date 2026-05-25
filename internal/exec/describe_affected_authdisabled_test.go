package exec

import (
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExecutePropagatesAuthDisabled pins the wiring that threads
// `DescribeAffectedCmdArgs.AuthDisabled` from the cmd layer all the way down to
// `executeDescribeAffectedWith*` and `addDependentsToAffected`. Before the fix, a nil
// AuthManager was indistinguishable from "no identity specified" downstream, so per-component
// auth resolution still ran even when the user passed `--identity=false`. See plan:
// --identity=false not honored in `atmos describe affected`.
func TestExecutePropagatesAuthDisabled(t *testing.T) {
	cases := []struct {
		name             string
		args             *DescribeAffectedCmdArgs
		expectFn         string // which executeDescribeAffectedWith* is expected to fire.
		expectDependents bool
	}{
		{
			name: "RepoPath path",
			args: &DescribeAffectedCmdArgs{
				Format:            "json",
				RepoPath:          "/tmp/anything",
				AuthDisabled:      true,
				IncludeDependents: true,
			},
			expectFn:         "repoPath",
			expectDependents: true,
		},
		{
			name: "CloneTargetRef path",
			args: &DescribeAffectedCmdArgs{
				Format:         "json",
				CloneTargetRef: true,
				AuthDisabled:   true,
			},
			expectFn: "clone",
		},
		{
			name: "Checkout path (default)",
			args: &DescribeAffectedCmdArgs{
				Format:       "json",
				AuthDisabled: true,
			},
			expectFn: "checkout",
		},
		{
			name: "AuthDisabled=false still propagates the value",
			args: &DescribeAffectedCmdArgs{
				Format:            "json",
				IncludeDependents: true,
				AuthDisabled:      false,
			},
			expectFn:         "checkout",
			expectDependents: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				gotRepoAuthDisabled       *bool
				gotCloneAuthDisabled      *bool
				gotCheckoutAuthDisabled   *bool
				gotDependentsAuthDisabled *bool
			)

			affectedResult := []schema.Affected{{Stack: "ue1-dev", Component: "vpc"}}

			d := describeAffectedExec{atmosConfig: &schema.AtmosConfiguration{}}
			d.IsTTYSupportForStdout = func() bool { return false }
			d.executeDescribeAffectedWithTargetRepoPath = func(_ *schema.AtmosConfiguration, _ string, _ bool, _ bool, _ string, _ bool, _ bool, _ []string, _ bool, _ auth.AuthManager, authDisabled bool) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
				v := authDisabled
				gotRepoAuthDisabled = &v
				return affectedResult, nil, nil, "", nil
			}
			d.executeDescribeAffectedWithTargetRefClone = func(_ *schema.AtmosConfiguration, _ string, _ string, _ string, _ string, _ bool, _ bool, _ string, _ bool, _ bool, _ []string, _ bool, _ auth.AuthManager, authDisabled bool) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
				v := authDisabled
				gotCloneAuthDisabled = &v
				return affectedResult, nil, nil, "", nil
			}
			d.executeDescribeAffectedWithTargetRefCheckout = func(_ *schema.AtmosConfiguration, _ string, _ string, _ string, _ bool, _ bool, _ string, _ bool, _ bool, _ []string, _ bool, _ auth.AuthManager, authDisabled bool) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
				v := authDisabled
				gotCheckoutAuthDisabled = &v
				return affectedResult, nil, nil, "", nil
			}
			d.addDependentsToAffected = func(_ *schema.AtmosConfiguration, _ *[]schema.Affected, _ bool, _ bool, _ bool, _ []string, _ string, _ auth.AuthManager, authDisabled bool) error {
				v := authDisabled
				gotDependentsAuthDisabled = &v
				return nil
			}
			d.printOrWriteToFile = func(_ *schema.AtmosConfiguration, _ string, _ string, _ any) error {
				return nil
			}

			err := d.Execute(tc.args)
			require.NoError(t, err)

			switch tc.expectFn {
			case "repoPath":
				require.NotNil(t, gotRepoAuthDisabled, "RepoPath helper must be called")
				assert.Equal(t, tc.args.AuthDisabled, *gotRepoAuthDisabled)
				assert.Nil(t, gotCloneAuthDisabled, "Clone helper must not be called")
				assert.Nil(t, gotCheckoutAuthDisabled, "Checkout helper must not be called")
			case "clone":
				require.NotNil(t, gotCloneAuthDisabled, "Clone helper must be called")
				assert.Equal(t, tc.args.AuthDisabled, *gotCloneAuthDisabled)
				assert.Nil(t, gotRepoAuthDisabled, "RepoPath helper must not be called")
				assert.Nil(t, gotCheckoutAuthDisabled, "Checkout helper must not be called")
			case "checkout":
				require.NotNil(t, gotCheckoutAuthDisabled, "Checkout helper must be called")
				assert.Equal(t, tc.args.AuthDisabled, *gotCheckoutAuthDisabled)
				assert.Nil(t, gotRepoAuthDisabled, "RepoPath helper must not be called")
				assert.Nil(t, gotCloneAuthDisabled, "Clone helper must not be called")
			}

			if tc.expectDependents {
				require.NotNil(t, gotDependentsAuthDisabled, "addDependentsToAffected must be called when IncludeDependents=true")
				assert.Equal(t, tc.args.AuthDisabled, *gotDependentsAuthDisabled,
					"AuthDisabled must be forwarded to addDependentsToAffected so the inner ExecuteDescribeStacksWithAuthDisabled receives it")
			} else {
				assert.Nil(t, gotDependentsAuthDisabled, "addDependentsToAffected must not be called when IncludeDependents=false")
			}
		})
	}
}
