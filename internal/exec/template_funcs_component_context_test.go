package exec

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestAtmosFuncsComponentCloudFormationContext checks the complete template
// function path, including cancellation during the provider lookup.
func TestAtmosFuncsComponentCloudFormationContext(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		canceled, expired, cancelDuring bool
		wantErr                         error
	}{
		{name: "already canceled", canceled: true, wantErr: context.Canceled},
		{name: "expired deadline", expired: true, wantErr: context.DeadlineExceeded},
		{name: "cancel during lookup", cancelDuring: true, wantErr: context.Canceled},
		{name: "live caller deadline"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearComponentFuncSyncMap(t)
			ac := setupAwsCloudFormationOutputFixture(t)
			deadline := time.Now().Add(time.Minute)
			if tc.expired {
				deadline = time.Now().Add(-time.Minute)
			}
			ctx, cancel := context.WithDeadline(context.Background(), deadline)
			defer cancel()
			if tc.canceled {
				cancel()
			}
			getter := NewMockCloudFormationOutputsGetter(gomock.NewController(t))
			var providerContext context.Context
			getter.EXPECT().GetOutputs(gomock.Any(), "us-east-1", "test-vpc", gomock.Any()).DoAndReturn(
				func(got context.Context, _, _ string, _ *schema.AWSAuthContext) (map[string]any, error) {
					providerContext = got
					actualDeadline, ok := got.Deadline()
					assert.True(t, ok)
					assert.Equal(t, deadline, actualDeadline, "caller deadline must bound the output request")
					if tc.cancelDuring {
						cancel()
					}
					assert.ErrorIs(t, got.Err(), tc.wantErr)
					return map[string]any{"VpcId": "vpc-test"}, got.Err()
				},
			)
			stubCloudFormationOutputsGetter(t, getter)
			funcs := AtmosFuncs{atmosConfig: &ac, ctx: ctx}
			_, err := funcs.Component("vpc", "test")
			require.ErrorIs(t, err, tc.wantErr)
			require.NotNil(t, providerContext)
			assert.Error(t, providerContext.Err(), "request context must be released after lookup")
			if tc.wantErr == nil {
				assert.NoError(t, ctx.Err(), "releasing request context must not cancel caller")
			}
		})
	}
}

// TestAtmosFuncsComponentCloudFormationTimeout retains the five-minute cap when
// the caller has no deadline or a later deadline.
func TestAtmosFuncsComponentCloudFormationTimeout(t *testing.T) {
	for _, laterDeadline := range []bool{false, true} {
		name := "no caller deadline"
		if laterDeadline {
			name = "later caller deadline"
		}
		t.Run(name, func(t *testing.T) {
			clearComponentFuncSyncMap(t)
			ac := setupAwsCloudFormationOutputFixture(t)
			ctx := t.Context()
			if laterDeadline {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 2*cloudFormationOutputsTimeout)
				defer cancel()
			}
			getter := NewMockCloudFormationOutputsGetter(gomock.NewController(t))
			started := time.Now()
			getter.EXPECT().GetOutputs(gomock.Any(), "us-east-1", "test-vpc", gomock.Any()).DoAndReturn(
				func(got context.Context, _, _ string, _ *schema.AWSAuthContext) (map[string]any, error) {
					deadline, ok := got.Deadline()
					require.True(t, ok)
					assert.WithinRange(t, deadline, started.Add(cloudFormationOutputsTimeout), time.Now().Add(cloudFormationOutputsTimeout))
					return map[string]any{"VpcId": "vpc-test"}, nil
				},
			)
			stubCloudFormationOutputsGetter(t, getter)
			funcs := AtmosFuncs{atmosConfig: &ac, ctx: ctx}
			_, err := funcs.Component("vpc", "test")
			require.NoError(t, err)
		})
	}
}
