package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

// shrinkStackSetTiming replaces the package's StackSet operation poll
// interval/timeout with tiny values for the duration of a test, restoring the
// originals on cleanup — mirrors shrinkDriftTiming in drift_test.go.
func shrinkStackSetTiming(t *testing.T, interval, timeout time.Duration) {
	t.Helper()
	origInterval := stackSetOperationPollInterval
	origTimeout := stackSetOperationTimeout
	stackSetOperationPollInterval = interval
	stackSetOperationTimeout = timeout
	t.Cleanup(func() {
		stackSetOperationPollInterval = origInterval
		stackSetOperationTimeout = origTimeout
	})
}

// resolveStackSetTarget must error when no `kind: aws/stackset` target is declared.
func TestResolveStackSetTarget_NoneDeclared(t *testing.T) {
	_, err := resolveStackSetTarget(map[string]any{}, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
}

// resolveStackSetTarget must implicitly select the single declared target
// when there's exactly one, with no --target needed.
func TestResolveStackSetTarget_ImplicitSingle(t *testing.T) {
	provisionSection := map[string]any{
		"targets": map[string]any{
			"multi-region": map[string]any{
				"kind":     kindAwsStackSet,
				"accounts": []any{"111111111111"},
				"regions":  []any{"us-east-1"},
			},
			"direct": map[string]any{
				"kind": "aws/s3",
			},
		},
	}

	cfg, err := resolveStackSetTarget(provisionSection, "")
	require.NoError(t, err)
	assert.Equal(t, "multi-region", cfg.Name)
	assert.Equal(t, []string{"111111111111"}, cfg.Accounts)
}

// resolveStackSetTarget must select the target named by --target when there
// are multiple `kind: aws/stackset` targets and it matches one of them.
func TestResolveStackSetTarget_MultipleWithMatchingFlag(t *testing.T) {
	provisionSection := map[string]any{
		"targets": map[string]any{
			"east": map[string]any{"kind": kindAwsStackSet, "regions": []any{"us-east-1"}},
			"west": map[string]any{"kind": kindAwsStackSet, "regions": []any{"us-west-2"}},
		},
	}

	cfg, err := resolveStackSetTarget(provisionSection, "west")
	require.NoError(t, err)
	assert.Equal(t, "west", cfg.Name)
	assert.Equal(t, []string{"us-west-2"}, cfg.Regions)
}

// resolveStackSetTarget must error when --target names something that isn't a
// declared `kind: aws/stackset` target (missing entirely, or a different kind).
func TestResolveStackSetTarget_FlagMatchesNone(t *testing.T) {
	provisionSection := map[string]any{
		"targets": map[string]any{
			"east": map[string]any{"kind": kindAwsStackSet},
		},
	}

	_, err := resolveStackSetTarget(provisionSection, "does-not-exist")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
}

// resolveStackSetTarget must error as ambiguous when multiple targets are
// declared and no --target was given to disambiguate.
func TestResolveStackSetTarget_MultipleNoFlagAmbiguous(t *testing.T) {
	provisionSection := map[string]any{
		"targets": map[string]any{
			"east": map[string]any{"kind": kindAwsStackSet},
			"west": map[string]any{"kind": kindAwsStackSet},
		},
	}

	_, err := resolveStackSetTarget(provisionSection, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
}

// stackSetConfigFromTarget must default PermissionModel to SELF_MANAGED when
// not set, and honor an explicit override otherwise; it must also extract
// administration_role_arn/execution_role_name only when present.
func TestStackSetConfigFromTarget(t *testing.T) {
	t.Run("default permission model, no roles", func(t *testing.T) {
		cfg := stackSetConfigFromTarget("mine", map[string]any{
			"accounts": []any{"111111111111", "222222222222"},
			"regions":  []any{"us-east-1"},
		})
		assert.Equal(t, "mine", cfg.Name)
		assert.Equal(t, defaultPermissionModel, cfg.PermissionModel)
		assert.Equal(t, []string{"111111111111", "222222222222"}, cfg.Accounts)
		assert.Equal(t, []string{"us-east-1"}, cfg.Regions)
		assert.Empty(t, cfg.AdministrationRoleArn)
		assert.Empty(t, cfg.ExecutionRoleName)
	})

	t.Run("explicit permission model override and roles", func(t *testing.T) {
		cfg := stackSetConfigFromTarget("mine", map[string]any{
			"permission_model":        "SERVICE_MANAGED",
			"administration_role_arn": "arn:aws:iam::111111111111:role/AdminRole",
			"execution_role_name":     "ExecutionRole",
		})
		assert.Equal(t, "SERVICE_MANAGED", cfg.PermissionModel)
		assert.Equal(t, "arn:aws:iam::111111111111:role/AdminRole", cfg.AdministrationRoleArn)
		assert.Equal(t, "ExecutionRole", cfg.ExecutionRoleName)
	})
}

// toStringSlice must normalize a []any of strings, return nil for nil/a
// non-[]any value, and silently drop mixed-type entries (documented behavior,
// asserted explicitly rather than merely "doesn't panic").
func TestToStringSlice(t *testing.T) {
	assert.Nil(t, toStringSlice(nil))
	assert.Nil(t, toStringSlice(""))
	assert.Nil(t, toStringSlice(42))
	assert.Equal(t, []string{"a", "b"}, toStringSlice([]any{"a", "b"}))
	assert.Equal(t, []string{"a", "c"}, toStringSlice([]any{"a", 42, "c", true}), "non-string entries must be silently dropped")
	assert.Equal(t, []string{"123456789012"}, toStringSlice("123456789012"), "a single scalar string is a one-element list, not silently dropped")
}

// runStackSetCreate must wrap a CreateStackSet API error and never attempt
// CreateStackInstances.
func TestRunStackSetCreate_CreateStackSetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().CreateStackSet(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))
	// No CreateStackInstances expectation: any call would fail via gomock's
	// unexpected-call panic.

	spec := &stackSpec{StackName: "vpc"}
	ssCfg := &stackSetConfig{Name: "mine", PermissionModel: defaultPermissionModel}
	_, err := runStackSetCreate(context.Background(), client, spec, ssCfg, map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// runStackSetCreate must stop after CreateStackSet (never call
// CreateStackInstances) when no accounts/regions are configured.
func TestRunStackSetCreate_NoAccountsRegions_SkipsInstances(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().CreateStackSet(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateStackSetOutput{}, nil)
	client.EXPECT().CreateStackInstances(gomock.Any(), gomock.Any()).Times(0)

	spec := &stackSpec{StackName: "vpc"}
	ssCfg := &stackSetConfig{Name: "mine", PermissionModel: defaultPermissionModel}

	out := captureStdout(t, func() {
		summary, err := runStackSetCreate(context.Background(), client, spec, ssCfg, map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, "vpc", summary["stackset_name"])
	})
	assert.Contains(t, out, "stackset created")
}

// runStackSetCreate must call both CreateStackSet and CreateStackInstances,
// in that order, when accounts/regions are declared.
func TestRunStackSetCreate_WithAccountsRegions_CallsInstancesCreate(t *testing.T) {
	shrinkStackSetTiming(t, time.Millisecond, time.Minute)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	gomock.InOrder(
		client.EXPECT().CreateStackSet(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateStackSetOutput{}, nil),
		client.EXPECT().CreateStackInstances(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateStackInstancesOutput{
			OperationId: awsString("op-1"),
		}, nil),
		client.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{
			StackSetOperation: &cfntypes.StackSetOperation{Status: cfntypes.StackSetOperationStatusSucceeded},
		}, nil),
	)

	spec := &stackSpec{StackName: "vpc"}
	ssCfg := &stackSetConfig{
		Name:            "mine",
		PermissionModel: defaultPermissionModel,
		Accounts:        []string{"111111111111"},
		Regions:         []string{"us-east-1"},
	}

	out := captureStdout(t, func() {
		summary, err := runStackSetCreate(context.Background(), client, spec, ssCfg, map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, string(cfntypes.StackSetOperationStatusSucceeded), summary["operation_status"])
	})
	assert.Contains(t, out, "stackset created")
	assert.Contains(t, out, "instance(s)")
}

// resolveStackSetTarget must skip (not panic on) a target block whose value
// isn't a map[string]any — a malformed provision.targets entry.
func TestResolveStackSetTarget_NonMapTargetValueSkipped(t *testing.T) {
	provisionSection := map[string]any{
		"targets": map[string]any{
			"malformed":    "not-a-map",
			"multi-region": map[string]any{"kind": kindAwsStackSet},
		},
	}

	cfg, err := resolveStackSetTarget(provisionSection, "")
	require.NoError(t, err)
	assert.Equal(t, "multi-region", cfg.Name)
}

// runStackSetInstancesCreate must wrap a CreateStackInstances API error.
func TestRunStackSetInstancesCreate_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().CreateStackInstances(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	ssCfg := &stackSetConfig{Accounts: []string{"111111111111"}, Regions: []string{"us-east-1"}}
	_, err := runStackSetInstancesCreate(context.Background(), client, "mine", ssCfg, map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// runStackSetInstancesCreate's happy path: creates instances then polls to
// SUCCEEDED.
func TestRunStackSetInstancesCreate_Success(t *testing.T) {
	shrinkStackSetTiming(t, time.Millisecond, time.Minute)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().CreateStackInstances(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateStackInstancesOutput{
		OperationId: awsString("op-1"),
	}, nil)
	client.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{
		StackSetOperation: &cfntypes.StackSetOperation{Status: cfntypes.StackSetOperationStatusSucceeded},
	}, nil)

	ssCfg := &stackSetConfig{Accounts: []string{"111111111111"}, Regions: []string{"us-east-1"}}
	out := captureStdout(t, func() {
		summary, err := runStackSetInstancesCreate(context.Background(), client, "mine", ssCfg, map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, string(cfntypes.StackSetOperationStatusSucceeded), summary["operation_status"])
	})
	assert.Contains(t, out, "mine:")
}

// runStackSetInstancesCreate must propagate a pollStackSetOperation failure
// (the operation itself reaching a FAILED terminal status) after
// CreateStackInstances succeeds.
func TestRunStackSetInstancesCreate_OperationPollFails(t *testing.T) {
	shrinkStackSetTiming(t, time.Millisecond, time.Minute)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().CreateStackInstances(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateStackInstancesOutput{
		OperationId: awsString("op-1"),
	}, nil)
	client.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{
		StackSetOperation: &cfntypes.StackSetOperation{Status: cfntypes.StackSetOperationStatusFailed},
	}, nil)

	ssCfg := &stackSetConfig{Accounts: []string{"111111111111"}, Regions: []string{"us-east-1"}}
	_, err := runStackSetInstancesCreate(context.Background(), client, "mine", ssCfg, map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationOperationFailed)
}

// runStackSetUpdate must wrap an UpdateStackSet API error.
func TestRunStackSetUpdate_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().UpdateStackSet(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	spec := &stackSpec{StackName: "vpc"}
	ssCfg := &stackSetConfig{PermissionModel: defaultPermissionModel}
	_, err := runStackSetUpdate(context.Background(), client, spec, ssCfg, map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// runStackSetUpdate's happy path: updates then polls to SUCCEEDED.
func TestRunStackSetUpdate_Success(t *testing.T) {
	shrinkStackSetTiming(t, time.Millisecond, time.Minute)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().UpdateStackSet(gomock.Any(), gomock.Any()).Return(&cloudformation.UpdateStackSetOutput{
		OperationId: awsString("op-1"),
	}, nil)
	client.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{
		StackSetOperation: &cfntypes.StackSetOperation{Status: cfntypes.StackSetOperationStatusSucceeded},
	}, nil)

	spec := &stackSpec{StackName: "vpc"}
	ssCfg := &stackSetConfig{PermissionModel: defaultPermissionModel}
	out := captureStdout(t, func() {
		summary, err := runStackSetUpdate(context.Background(), client, spec, ssCfg, map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, string(cfntypes.StackSetOperationStatusSucceeded), summary["operation_status"])
	})
	assert.Contains(t, out, "stackset updated")
}

// runStackSetUpdate must return an error wrapping
// ErrAwsCloudFormationOperationFailed when the operation polls to FAILED.
func TestRunStackSetUpdate_OperationFails(t *testing.T) {
	shrinkStackSetTiming(t, time.Millisecond, time.Minute)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().UpdateStackSet(gomock.Any(), gomock.Any()).Return(&cloudformation.UpdateStackSetOutput{
		OperationId: awsString("op-1"),
	}, nil)
	client.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{
		StackSetOperation: &cfntypes.StackSetOperation{Status: cfntypes.StackSetOperationStatusFailed},
	}, nil)

	spec := &stackSpec{StackName: "vpc"}
	ssCfg := &stackSetConfig{PermissionModel: defaultPermissionModel}
	_, err := runStackSetUpdate(context.Background(), client, spec, ssCfg, map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationOperationFailed)
}

// runStackSetDelete must skip DeleteStackInstances entirely (going straight
// to DeleteStackSet) when the StackSet has zero instances.
func TestRunStackSetDelete_ZeroInstances_SkipsDeleteInstances(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStackInstancesOutput{}, nil)
	client.EXPECT().DeleteStackInstances(gomock.Any(), gomock.Any()).Times(0)
	client.EXPECT().DeleteStackSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteStackSetOutput{}, nil)

	out := captureStdout(t, func() {
		summary, err := runStackSetDelete(context.Background(), client, "mine", map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, "mine", summary["stackset_name"])
	})
	assert.Contains(t, out, "stackset deleted")
}

// runStackSetDelete must call DeleteStackInstances with the right
// accounts/regions before DeleteStackSet when the StackSet has instances.
func TestRunStackSetDelete_WithInstances_CallsDeleteInstances(t *testing.T) {
	shrinkStackSetTiming(t, time.Millisecond, time.Minute)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStackInstancesOutput{
		Summaries: []cfntypes.StackInstanceSummary{
			{Account: awsString("111111111111"), Region: awsString("us-east-1")},
		},
	}, nil)

	var gotAccounts, gotRegions []string
	client.EXPECT().DeleteStackInstances(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input *cloudformation.DeleteStackInstancesInput, _ ...func(*cloudformation.Options)) (*cloudformation.DeleteStackInstancesOutput, error) {
			gotAccounts = input.Accounts
			gotRegions = input.Regions
			return &cloudformation.DeleteStackInstancesOutput{OperationId: awsString("op-1")}, nil
		},
	)
	client.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{
		StackSetOperation: &cfntypes.StackSetOperation{Status: cfntypes.StackSetOperationStatusSucceeded},
	}, nil)
	client.EXPECT().DeleteStackSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteStackSetOutput{}, nil)

	_, err := runStackSetDelete(context.Background(), client, "mine", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, []string{"111111111111"}, gotAccounts)
	assert.Equal(t, []string{"us-east-1"}, gotRegions)
}

// runStackSetDelete must wrap a DeleteStackInstances API error.
func TestRunStackSetDelete_DeleteInstancesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStackInstancesOutput{
		Summaries: []cfntypes.StackInstanceSummary{
			{Account: awsString("111111111111"), Region: awsString("us-east-1")},
		},
	}, nil)
	client.EXPECT().DeleteStackInstances(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := runStackSetDelete(context.Background(), client, "mine", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// runStackSetDelete must wrap a DeleteStackSet API error.
func TestRunStackSetDelete_DeleteStackSetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStackInstancesOutput{}, nil)
	client.EXPECT().DeleteStackSet(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := runStackSetDelete(context.Background(), client, "mine", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// runStackSetDelete must propagate a listStackSetInstances (ListStackInstances)
// API error before ever attempting DeleteStackInstances/DeleteStackSet.
func TestRunStackSetDelete_ListInstancesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))
	// No DeleteStackInstances/DeleteStackSet expectations: a call would fail
	// via gomock's unexpected-call panic, proving the error short-circuited.

	_, err := runStackSetDelete(context.Background(), client, "mine", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// runStackSetDelete must propagate a pollStackSetOperation failure (the
// DeleteStackInstances operation reaching a FAILED terminal status) before
// ever attempting DeleteStackSet.
func TestRunStackSetDelete_OperationPollFails(t *testing.T) {
	shrinkStackSetTiming(t, time.Millisecond, time.Minute)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStackInstancesOutput{
		Summaries: []cfntypes.StackInstanceSummary{
			{Account: awsString("111111111111"), Region: awsString("us-east-1")},
		},
	}, nil)
	client.EXPECT().DeleteStackInstances(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteStackInstancesOutput{
		OperationId: awsString("op-1"),
	}, nil)
	client.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{
		StackSetOperation: &cfntypes.StackSetOperation{Status: cfntypes.StackSetOperationStatusFailed},
	}, nil)
	client.EXPECT().DeleteStackSet(gomock.Any(), gomock.Any()).Times(0)

	_, err := runStackSetDelete(context.Background(), client, "mine", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationOperationFailed)
}

// runStackSetInstances must propagate a listStackSetInstances failure.
func TestRunStackSetInstances_ListError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := runStackSetInstances(context.Background(), client, "mine", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// runStackSetInstances must render a "no stack instances" line when the
// StackSet has none.
func TestRunStackSetInstances_Empty(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStackInstancesOutput{}, nil)

	out := captureStdout(t, func() {
		summary, err := runStackSetInstances(context.Background(), client, "mine", map[string]any{})
		require.NoError(t, err)
		assert.Empty(t, summary["instances"])
	})
	assert.Contains(t, out, "mine: no stack instances")
}

// runStackSetInstances must render the summary line for each populated
// instance.
func TestRunStackSetInstances_Populated(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStackInstancesOutput{
		Summaries: []cfntypes.StackInstanceSummary{
			{
				Account: awsString("111111111111"),
				Region:  awsString("us-east-1"),
				Status:  cfntypes.StackInstanceStatusCurrent,
				StackId: awsString("arn:aws:cloudformation:us-east-1:111111111111:stack/mine/abc"),
			},
		},
	}, nil)

	out := captureStdout(t, func() {
		summary, err := runStackSetInstances(context.Background(), client, "mine", map[string]any{})
		require.NoError(t, err)
		assert.Len(t, summary["instances"].([]cfntypes.StackInstanceSummary), 1)
	})
	assert.Contains(t, out, "111111111111")
	assert.Contains(t, out, "us-east-1")
	assert.Contains(t, out, string(cfntypes.StackInstanceStatusCurrent))
}

// listStackSetInstances must return a single page as-is, and must paginate
// across 2+ pages via NextToken, accumulating every page's instances (one
// per page, in order) across the whole chain. Each subtest's page count
// drives a loop that builds the mock's call chain dynamically via .After,
// rather than a fixed literal sequence.
func TestListStackSetInstances_Pagination(t *testing.T) {
	tests := []struct {
		name      string
		pageCount int
	}{
		{"single page", 1},
		{"paginates across pages", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockCloudFormationClient(ctrl)

			var wantAccounts []string
			var prevCall *gomock.Call
			for page := 1; page <= tt.pageCount; page++ {
				account := fmt.Sprintf("%012d", page)
				wantAccounts = append(wantAccounts, account)

				out := &cloudformation.ListStackInstancesOutput{
					Summaries: []cfntypes.StackInstanceSummary{{Account: awsString(account)}},
				}
				isLastPage := page == tt.pageCount
				if !isLastPage {
					token := fmt.Sprintf("token-%d", page)
					out.NextToken = &token
				}

				thisCall := client.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(out, nil)
				if prevCall != nil {
					thisCall.After(prevCall)
				}
				prevCall = thisCall
			}

			instances, err := listStackSetInstances(context.Background(), client, "mine")
			require.NoError(t, err)
			require.Len(t, instances, len(wantAccounts))
			for i, wantAccount := range wantAccounts {
				assert.Equal(t, wantAccount, stringValue(instances[i].Account))
			}
		})
	}
}

// listStackSetInstances must wrap a ListStackInstances API error.
func TestListStackSetInstances_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := listStackSetInstances(context.Background(), client, "mine")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// instanceAccountsRegions/mapKeys must dedup accounts/regions shared across
// instances, and return empty slices (not panic) for empty input.
func TestInstanceAccountsRegions(t *testing.T) {
	t.Run("dedups across instances", func(t *testing.T) {
		instances := []cfntypes.StackInstanceSummary{
			{Account: awsString("111111111111"), Region: awsString("us-east-1")},
			{Account: awsString("111111111111"), Region: awsString("us-west-2")},
			{Account: awsString("222222222222"), Region: awsString("us-east-1")},
		}
		accounts, regions := instanceAccountsRegions(instances)
		assert.ElementsMatch(t, []string{"111111111111", "222222222222"}, accounts)
		assert.ElementsMatch(t, []string{"us-east-1", "us-west-2"}, regions)
	})

	t.Run("empty input", func(t *testing.T) {
		accounts, regions := instanceAccountsRegions(nil)
		assert.Empty(t, accounts)
		assert.Empty(t, regions)
	})
}

// pollStackSetOperation must return immediately on SUCCEEDED.
func TestPollStackSetOperation_Succeeded(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{
		StackSetOperation: &cfntypes.StackSetOperation{Status: cfntypes.StackSetOperationStatusSucceeded},
	}, nil)

	status, err := pollStackSetOperation(context.Background(), client, "mine", "op-1")
	require.NoError(t, err)
	assert.Equal(t, cfntypes.StackSetOperationStatusSucceeded, status)
}

// pollStackSetOperation must return a wrapped error on FAILED.
func TestPollStackSetOperation_Failed(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{
		StackSetOperation: &cfntypes.StackSetOperation{Status: cfntypes.StackSetOperationStatusFailed},
	}, nil)

	_, err := pollStackSetOperation(context.Background(), client, "mine", "op-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationOperationFailed)
}

// pollStackSetOperation must return a wrapped error on STOPPED.
func TestPollStackSetOperation_Stopped(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{
		StackSetOperation: &cfntypes.StackSetOperation{Status: cfntypes.StackSetOperationStatusStopped},
	}, nil)

	_, err := pollStackSetOperation(context.Background(), client, "mine", "op-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationOperationFailed)
}

// pollStackSetOperation must keep polling (not return) while RUNNING, then
// return once it reaches SUCCEEDED.
func TestPollStackSetOperation_RunningThenSucceeded(t *testing.T) {
	shrinkStackSetTiming(t, time.Millisecond, time.Minute)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	gomock.InOrder(
		client.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{
			StackSetOperation: &cfntypes.StackSetOperation{Status: cfntypes.StackSetOperationStatusRunning},
		}, nil),
		client.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{
			StackSetOperation: &cfntypes.StackSetOperation{Status: cfntypes.StackSetOperationStatusSucceeded},
		}, nil),
	)

	status, err := pollStackSetOperation(context.Background(), client, "mine", "op-1")
	require.NoError(t, err)
	assert.Equal(t, cfntypes.StackSetOperationStatusSucceeded, status)
}

// pollStackSetOperation must return ctx.Err() when the context is cancelled
// while waiting between polls.
func TestPollStackSetOperation_ContextCancelled(t *testing.T) {
	shrinkStackSetTiming(t, time.Minute, time.Hour)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{
		StackSetOperation: &cfntypes.StackSetOperation{Status: cfntypes.StackSetOperationStatusRunning},
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := pollStackSetOperation(ctx, client, "mine", "op-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// pollStackSetOperation must give up once the deadline passes, without ever
// reaching a terminal status.
func TestPollStackSetOperation_Timeout(t *testing.T) {
	shrinkStackSetTiming(t, time.Millisecond, time.Millisecond)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{
		StackSetOperation: &cfntypes.StackSetOperation{Status: cfntypes.StackSetOperationStatusRunning},
	}, nil).AnyTimes()

	_, err := pollStackSetOperation(context.Background(), client, "mine", "op-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationOperationFailed)
	assert.Contains(t, err.Error(), "timed out")
}

// pollStackSetOperation must wrap a DescribeStackSetOperation API error.
func TestPollStackSetOperation_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied"))

	_, err := pollStackSetOperation(context.Background(), client, "mine", "op-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// pollStackSetOperation must error (not nil-pointer-dereference) when
// DescribeStackSetOperation returns a nil StackSetOperation.
func TestPollStackSetOperation_NilOperation(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{}, nil)

	_, err := pollStackSetOperation(context.Background(), client, "mine", "op-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// nilIfEmpty must return nil for an empty string and a pointer to the value
// for a non-empty one.
func TestNilIfEmpty(t *testing.T) {
	assert.Nil(t, nilIfEmpty(""))
	got := nilIfEmpty("value")
	require.NotNil(t, got)
	assert.Equal(t, "value", *got)
}
