package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDescribeDependentsExec_Execute_ForwardsAuthDisabled pins the propagation of
// `DescribeDependentsExecProps.AuthDisabled` → `DescribeDependentsArgs.AuthDisabled`.
// Without this propagation, even though `cmd/describe_dependents.go` records the
// disabled identity signal in `props.AuthDisabled`, the executor would drop it on the
// floor and the inner `ExecuteDescribeStacksWithAuthDisabled` call would always see
// `authDisabled=false` — re-introducing the per-component auth attempt the user tried
// to disable. CodeRabbit flagged this gap on PR #2471.
func TestDescribeDependentsExec_Execute_ForwardsAuthDisabled(t *testing.T) {
	cases := []struct {
		name              string
		propsAuthDisabled bool
	}{
		{name: "AuthDisabled=true propagates", propsAuthDisabled: true},
		{name: "AuthDisabled=false propagates", propsAuthDisabled: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			var capturedArgs *DescribeDependentsArgs
			ex := &describeDependentsExec{
				atmosConfig: &schema.AtmosConfiguration{},
				executeDescribeDependents: func(_ *schema.AtmosConfiguration, args *DescribeDependentsArgs) ([]schema.Dependent, error) {
					capturedArgs = args
					return []schema.Dependent{}, nil
				},
				newPageCreator:        pager.NewMockPageCreator(ctrl),
				isTTYSupportForStdout: func() bool { return false },
				evaluateYqExpression: func(_ *schema.AtmosConfiguration, data any, _ string) (any, error) {
					return data, nil
				},
			}

			err := ex.Execute(&DescribeDependentsExecProps{
				Component:    "test-component",
				Stack:        "test-stack",
				Format:       "json",
				AuthDisabled: tc.propsAuthDisabled,
			})
			require.NoError(t, err)
			require.NotNil(t, capturedArgs, "executeDescribeDependents was not called")
			assert.Equal(t, tc.propsAuthDisabled, capturedArgs.AuthDisabled,
				"props.AuthDisabled must be forwarded to DescribeDependentsArgs.AuthDisabled")
		})
	}
}
