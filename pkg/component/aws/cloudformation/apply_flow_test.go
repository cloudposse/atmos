package cloudformation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	cfg "github.com/cloudposse/atmos/pkg/config"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stubStdinTerminal fakes whether stdin is a terminal for a single test.
func stubStdinTerminal(t *testing.T, isTerminal bool) {
	t.Helper()
	original := stdinIsTerminal
	stdinIsTerminal = func() bool { return isTerminal }
	t.Cleanup(func() { stdinIsTerminal = original })
}

// stackWithStatus is a DescribeStacks reply for a stack in the given status.
func stackWithStatus(status cfntypes.StackStatus) *cloudformation.DescribeStacksOutput {
	return &cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{StackStatus: status}}}
}

// addBucketChange is a changeset that would add one S3 bucket.
func addBucketChange() []cfntypes.Change {
	return []cfntypes.Change{{
		Type: cfntypes.ChangeTypeResource,
		ResourceChange: &cfntypes.ResourceChange{
			Action:            cfntypes.ChangeActionAdd,
			ResourceType:      awsString("AWS::S3::Bucket"),
			LogicalResourceId: awsString("Bucket"),
		},
	}}
}

// expectChangeSetCreated models createChangeSet for a stack in the given state
// (nil means CloudFormation does not know the stack at all) and returns the
// calls in order, for the caller to extend.
func expectChangeSetCreated(client *MockCloudFormationClient, stack *cloudformation.DescribeStacksOutput, described *cloudformation.DescribeChangeSetOutput) []any {
	if stack == nil {
		stack = &cloudformation.DescribeStacksOutput{}
	}
	return []any{
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(stack, nil),
		client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateChangeSetOutput{}, nil),
		client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(described, nil),
	}
}

func completedChangeSet() *cloudformation.DescribeChangeSetOutput {
	return &cloudformation.DescribeChangeSetOutput{Status: cfntypes.ChangeSetStatusCreateComplete, Changes: addBucketChange()}
}

// expectStubCleanup models discarding a changeset plus the empty stack a CREATE
// changeset registered: delete the changeset, confirm the stack is still
// REVIEW_IN_PROGRESS, then delete it.
func expectStubCleanup(client *MockCloudFormationClient) []any {
	return []any{
		client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(stackWithStatus(cfntypes.StackStatusReviewInProgress), nil),
		client.EXPECT().DeleteStack(gomock.Any(), &cloudformation.DeleteStackInput{StackName: awsString("vpc")}).Return(&cloudformation.DeleteStackOutput{}, nil),
	}
}

func vpcSpec() *stackSpec {
	return &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}
}

// H3: diff/plan on a never-deployed stack used to leave an empty
// REVIEW_IN_PROGRESS stack behind, because only the changeset was deleted.
func TestRunDiff_RemovesStubStackItCreated(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	calls := expectChangeSetCreated(client, nil, completedChangeSet())
	gomock.InOrder(append(calls, expectStubCleanup(client)...)...)

	_, err := runDiff(&opContext{Ctx: context.Background()}, client, vpcSpec(), map[string]any{})
	require.NoError(t, err)
}

// Negative path: a stack that already existed is never Atmos's to delete, even
// when it is a leftover REVIEW_IN_PROGRESS stub from an earlier invocation. Only
// the preview changeset goes (the mock has no DescribeStacks/DeleteStack call
// after the changeset delete).
func TestRunDiff_NeverDeletesPreExistingStack(t *testing.T) {
	for _, status := range []cfntypes.StackStatus{cfntypes.StackStatusReviewInProgress, cfntypes.StackStatusCreateComplete, cfntypes.StackStatusUpdateComplete} {
		t.Run(string(status), func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			calls := expectChangeSetCreated(client, stackWithStatus(status), completedChangeSet())
			gomock.InOrder(append(calls, client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, nil))...)

			_, err := runDiff(&opContext{Ctx: context.Background()}, client, vpcSpec(), map[string]any{})
			require.NoError(t, err)
		})
	}
}

// Negative path: even for a stub Atmos made, the stack is deleted only while it
// is still REVIEW_IN_PROGRESS. Any other status means it holds real resources.
func TestRunDiff_StubStackGuardChecksStatusFirst(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	calls := expectChangeSetCreated(client, nil, completedChangeSet())
	gomock.InOrder(append(
		calls,
		client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(stackWithStatus(cfntypes.StackStatusCreateComplete), nil),
	)...)

	_, err := runDiff(&opContext{Ctx: context.Background()}, client, vpcSpec(), map[string]any{})
	require.NoError(t, err)
}

// A stub cleanup failure is a warning, never a command failure.
func TestRunDiff_StubCleanupFailureIsNonFatal(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	calls := expectChangeSetCreated(client, nil, completedChangeSet())
	gomock.InOrder(append(
		calls,
		client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(stackWithStatus(cfntypes.StackStatusReviewInProgress), nil),
		client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied")),
	)...)

	_, err := runDiff(&opContext{Ctx: context.Background()}, client, vpcSpec(), map[string]any{})
	require.NoError(t, err)
}

// A changeset that fails to compute on a never-deployed stack abandons the
// CREATE changeset too, so the stub goes with it.
func TestRunDiff_FailedChangeSetRemovesStub(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	failed := &cloudformation.DescribeChangeSetOutput{Status: cfntypes.ChangeSetStatusFailed, StatusReason: awsString("Template format error: unsupported structure.")}
	calls := expectChangeSetCreated(client, nil, failed)
	gomock.InOrder(append(calls, expectStubCleanup(client)...)...)

	_, err := runDiff(&opContext{Ctx: context.Background()}, client, vpcSpec(), map[string]any{})
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
}

// M1: apply shows the predicted changes before asking, and asks only then.
func TestDeployDirect_PreviewPrecedesConfirmation(t *testing.T) {
	stubStdinTerminal(t, true)
	previewed := false
	var prompt string
	originalConfirm := confirmOperation
	confirmOperation = func(message string) (bool, error) {
		assert.True(t, previewed, "the changeset must exist and be computed before the prompt")
		prompt = message
		return true, nil
	}
	t.Cleanup(func() { confirmOperation = originalConfirm })

	client := NewMockCloudFormationClient(gomock.NewController(t))
	oldInterval := eventPollInterval
	eventPollInterval = time.Millisecond
	t.Cleanup(func() { eventPollInterval = oldInterval })
	stackEvent := stackLevelEvent("done", cfntypes.ResourceStatusUpdateComplete)
	gomock.InOrder(
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(stackWithStatus(cfntypes.StackStatusUpdateComplete), nil),
		client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateChangeSetOutput{}, nil),
		client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).DoAndReturn(
			func(context.Context, *cloudformation.DescribeChangeSetInput, ...func(*cloudformation.Options)) (*cloudformation.DescribeChangeSetOutput, error) {
				previewed = true
				return completedChangeSet(), nil
			},
		),
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil),
		client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.ExecuteChangeSetOutput{}, nil),
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{StackEvents: []cfntypes.StackEvent{stackEvent}}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(stackWithStatus(cfntypes.StackStatusUpdateComplete), nil),
	)

	octx := &opContext{Ctx: context.Background(), Flags: map[string]any{}}
	var err error
	out := captureStderr(t, func() { _, err = deployDirect(octx, client, vpcSpec()) })
	require.NoError(t, err)
	assert.Contains(t, prompt, `"vpc"`)
	preview := normalizeUIOutput(out)
	assert.Contains(t, preview, "vpc: 1 resource change(s)")
	assert.Contains(t, preview, "Add AWS::S3::Bucket Bucket")
}

// A declined prompt discards the changeset and, for a brand-new stack, the empty
// stub it registered, and reports a clear abort rather than a failure.
func TestDeployDirect_DeclinedDiscardsChangeSetAndStub(t *testing.T) {
	stubConfirmOperation(t, false, nil)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	calls := expectChangeSetCreated(client, nil, completedChangeSet())
	// No ExecuteChangeSet expectation: executing after a decline fails the test.
	gomock.InOrder(append(calls, expectStubCleanup(client)...)...)

	_, err := deployDirect(&opContext{Ctx: context.Background(), Flags: map[string]any{}}, client, vpcSpec())
	require.ErrorIs(t, err, errUtils.ErrUserAborted)
	details := cockroachErrors.GetAllDetails(err)
	require.NotEmpty(t, details)
	assert.Contains(t, strings.Join(details, " "), "No changes were made")
}

// Negative path: declining an update of an existing stack deletes only the
// changeset; the stack is untouched.
func TestDeployDirect_DeclinedKeepsExistingStack(t *testing.T) {
	stubConfirmOperation(t, false, nil)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	calls := expectChangeSetCreated(client, stackWithStatus(cfntypes.StackStatusUpdateComplete), completedChangeSet())
	gomock.InOrder(append(calls, client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, nil))...)

	_, err := deployDirect(&opContext{Ctx: context.Background(), Flags: map[string]any{}}, client, vpcSpec())
	require.ErrorIs(t, err, errUtils.ErrUserAborted)
}

// --auto-approve (deploy's default) skips the prompt; the preview still shows.
func TestDeployDirect_AutoApproveSkipsPrompt(t *testing.T) {
	originalConfirm := confirmOperation
	confirmOperation = func(string) (bool, error) {
		t.Fatal("--auto-approve must not prompt")
		return false, nil
	}
	t.Cleanup(func() { confirmOperation = originalConfirm })
	client := NewMockCloudFormationClient(gomock.NewController(t))
	expectDeployDirectFlow(t, client, cfntypes.StackStatusCreateComplete)

	var err error
	out := captureStderr(t, func() { _, err = deployDirect(autoApproveOctx(), client, vpcSpec()) })
	require.NoError(t, err)
	assert.Contains(t, normalizeUIOutput(out), "vpc:")
}

// M4: a no-op apply deletes the FAILED "didn't contain changes" changeset, says
// so, never asks, and never executes.
func TestDeployDirect_NoOpAnnouncesAndCleansUp(t *testing.T) {
	originalConfirm := confirmOperation
	confirmOperation = func(string) (bool, error) {
		t.Fatal("a no-op must not prompt")
		return false, nil
	}
	t.Cleanup(func() { confirmOperation = originalConfirm })
	client := NewMockCloudFormationClient(gomock.NewController(t))
	noOp := &cloudformation.DescribeChangeSetOutput{Status: cfntypes.ChangeSetStatusFailed, StatusReason: awsString("The submitted information didn't contain changes.")}
	calls := expectChangeSetCreated(client, stackWithStatus(cfntypes.StackStatusUpdateComplete), noOp)
	gomock.InOrder(append(calls, client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, nil))...)

	var result *changeSetResult
	var err error
	out := captureStderr(t, func() {
		result, err = deployDirect(&opContext{Ctx: context.Background(), Flags: map[string]any{}}, client, vpcSpec())
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.NoOp)
	assert.Contains(t, normalizeUIOutput(out), "No changes")
}

// changeset create on a no-op deletes the FAILED changeset and reports that none
// was kept; a real changeset is kept and its name printed.
func TestRunChangesetCreate_NoOpIsNotKept(t *testing.T) {
	noOp := &cloudformation.DescribeChangeSetOutput{Status: cfntypes.ChangeSetStatusFailed, StatusReason: awsString("The submitted information didn't contain changes.")}
	tests := []struct {
		name      string
		described *cloudformation.DescribeChangeSetOutput
		wantKept  bool
	}{
		{name: "no-op", described: noOp, wantKept: false},
		{name: "changes", described: completedChangeSet(), wantKept: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			calls := expectChangeSetCreated(client, stackWithStatus(cfntypes.StackStatusUpdateComplete), tt.described)
			if !tt.wantKept {
				calls = append(calls, client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, nil))
			}
			gomock.InOrder(calls...)

			var summary map[string]any
			var err error
			var stderr string
			stdout := captureStdout(t, func() {
				stderr = captureStderr(t, func() {
					summary, err = runChangesetCreate(&opContext{Ctx: context.Background()}, client, vpcSpec(), map[string]any{})
				})
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantKept, summary["changeset_kept"])
			if tt.wantKept {
				assert.Contains(t, stdout, "changeset: atmos-vpc-")
				return
			}
			assert.NotContains(t, stdout, "changeset: ")
			assert.Contains(t, normalizeUIOutput(stderr), "No changes; changeset not kept")
		})
	}
}

// A failed CREATE changeset from `changeset create` is cleaned up with its stub.
func TestRunChangesetCreate_FailureRemovesStub(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	failed := &cloudformation.DescribeChangeSetOutput{Status: cfntypes.ChangeSetStatusFailed, StatusReason: awsString("Template format error.")}
	calls := expectChangeSetCreated(client, nil, failed)
	gomock.InOrder(append(calls, expectStubCleanup(client)...)...)

	_, err := runChangesetCreate(&opContext{Ctx: context.Background()}, client, vpcSpec(), map[string]any{})
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
}

// A successful `changeset create` on a never-deployed stack must keep both the
// changeset and its REVIEW_IN_PROGRESS stack: that is the explicit, reusable
// artifact the user asked for (the mock has no delete calls).
func TestRunChangesetCreate_KeepsStubForExplicitChangeSet(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	gomock.InOrder(expectChangeSetCreated(client, nil, completedChangeSet())...)

	_ = captureStdout(t, func() {
		_, err := runChangesetCreate(&opContext{Ctx: context.Background()}, client, vpcSpec(), map[string]any{})
		require.NoError(t, err)
	})
}

// M3: a ROLLBACK_COMPLETE stack cannot be updated, so apply and diff fail before
// creating a changeset, with a recovery hint, and never delete anything.
func TestCreateChangeSet_RollbackCompleteFailsBeforeChangeSet(t *testing.T) {
	for name, run := range map[string]func(client CloudFormationClient) error{
		"diff": func(client CloudFormationClient) error {
			_, err := runDiff(&opContext{Ctx: context.Background()}, client, identifiedVpcSpec(), map[string]any{})
			return err
		},
		"apply": func(client CloudFormationClient) error {
			_, err := deployDirect(autoApproveOctx(), client, identifiedVpcSpec())
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			// Only DescribeStacks: CreateChangeSet/DeleteStack calls fail the test.
			client := NewMockCloudFormationClient(gomock.NewController(t))
			client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(stackWithStatus(cfntypes.StackStatusRollbackComplete), nil)

			err := run(client)
			require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationStackRollbackComplete)
			hints := strings.Join(cockroachErrors.GetAllHints(err), "\n")
			assert.Contains(t, hints, "atmos aws cloudformation delete vpc -s dev")
			assert.NotContains(t, hints, "<component>")
			assert.Contains(t, strings.Join(cockroachErrors.GetAllDetails(err), "\n"), "initial create failed")
		})
	}
}

// Negative path: UPDATE_ROLLBACK_COMPLETE is updatable, so it is not blocked.
func TestCreateChangeSet_UpdateRollbackCompleteIsUpdatable(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	gomock.InOrder(expectChangeSetCreated(client, stackWithStatus(cfntypes.StackStatusUpdateRollbackComplete), completedChangeSet())...)

	result, err := createChangeSet(context.Background(), client, vpcSpec())
	require.NoError(t, err)
	assert.Equal(t, cfntypes.ChangeSetTypeUpdate, result.ChangeSetType)
	assert.False(t, result.StackStub)
}

// stackStub marks only a CREATE changeset against a stack CloudFormation did not
// know at all.
func TestCreateChangeSet_StackStubFlag(t *testing.T) {
	tests := []struct {
		name  string
		stack *cloudformation.DescribeStacksOutput
		want  bool
	}{
		{name: "unknown stack", stack: nil, want: true},
		{name: "leftover REVIEW_IN_PROGRESS stack", stack: stackWithStatus(cfntypes.StackStatusReviewInProgress), want: false},
		{name: "existing stack", stack: stackWithStatus(cfntypes.StackStatusCreateComplete), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			gomock.InOrder(expectChangeSetCreated(client, tt.stack, completedChangeSet())...)
			result, err := createChangeSet(context.Background(), client, vpcSpec())
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.StackStub)
		})
	}
}

// M4: a stack named "atmos-..." no longer yields "atmos-atmos-...".
func TestChangeSetName_DoesNotDoubleAtmosPrefix(t *testing.T) {
	tests := []struct {
		stack      string
		wantPrefix string
	}{
		{stack: "atmos-tip-1550-bk-probe", wantPrefix: "atmos-tip-1550-bk-probe-"},
		{stack: "vpc", wantPrefix: "atmos-vpc-"},
		{stack: "atmosphere", wantPrefix: "atmos-atmosphere-"},
	}
	for _, tt := range tests {
		t.Run(tt.stack, func(t *testing.T) {
			name := changeSetName(tt.stack)
			assert.True(t, strings.HasPrefix(name, tt.wantPrefix), name)
			assert.NotContains(t, name, "atmos-atmos-")
			assert.LessOrEqual(t, len(name), changeSetNameMaxLength)
		})
	}
}

// A maximum-length "atmos-" stack name still fits CloudFormation's limit.
func TestChangeSetName_MaxLengthAtmosPrefixedStack(t *testing.T) {
	name := changeSetName("atmos-" + strings.Repeat("a", 122))
	assert.LessOrEqual(t, len(name), changeSetNameMaxLength)
	assert.True(t, strings.HasPrefix(name, "atmos-"))
}

// directApplyOctx builds an apply context with the default (implicit) target.
func directApplyOctx(flags map[string]any) *opContext {
	return &opContext{
		Ctx:         context.Background(),
		AtmosConfig: &schema.AtmosConfiguration{},
		Info:        &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc", ComponentSection: map[string]any{}},
		Flags:       flags,
	}
}

// publishOnlyOctx builds an apply context whose only target is an aws/s3 one.
func publishOnlyOctx(flags map[string]any) *opContext {
	return &opContext{
		Ctx:         context.Background(),
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
			ComponentSection: map[string]any{cfg.ProvisionSectionName: map[string]any{
				"targets": map[string]any{"artifacts": map[string]any{"kind": kindAwsS3, "bucket": "my-bucket", "region": "us-east-1"}},
			}},
		},
		Flags: flags,
	}
}

// Without --auto-approve and without a terminal, a direct deploy fails fast with
// the dedicated sentinel (not "user aborted") before packaging uploads anything
// or a changeset exists: the mock client has no expectations.
func TestDeliverApply_NonTTYWithoutAutoApproveFailsFast(t *testing.T) {
	stubStdinTerminal(t, false)
	client := NewMockCloudFormationClient(gomock.NewController(t))

	summary, result, err := deliverApply(directApplyOctx(map[string]any{}), client, vpcSpec())
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationConfirmationRequired)
	assert.NotErrorIs(t, err, errUtils.ErrUserAborted)
	assert.Nil(t, result)
	assert.True(t, errUtils.HasHint(err, "--auto-approve"))
	assert.Equal(t, "default", summary[targetKey])
}

// H4: a publish-only apply (an aws/s3 target) reports where the template went,
// carries the locations in the summary, and asks no stack-change question even
// without --auto-approve and without a terminal.
func TestDeliverApply_PublishOnlyReportsLocationAndNeverPrompts(t *testing.T) {
	stubStdinTerminal(t, false)
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl) // any CloudFormation call fails the test.
	backend := artifact.NewMockBackend(ctrl)
	backend.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	stubNewS3Backend(t, backend, nil)

	var summary map[string]any
	var err error
	out := captureStderr(t, func() {
		summary, _, err = deliverApply(publishOnlyOctx(map[string]any{targetKey: "artifacts"}), client, vpcSpec())
	})
	require.NoError(t, err)

	httpsURL, _ := summary[packageURLKey].(string)
	s3URI, _ := summary[packageS3URIKey].(string)
	require.NotEmpty(t, httpsURL)
	require.True(t, strings.HasPrefix(s3URI, "s3://my-bucket/"), s3URI)
	assert.True(t, strings.HasPrefix(httpsURL, "https://my-bucket.s3.us-east-1.amazonaws.com/"), httpsURL)

	reported := normalizeUIOutput(out)
	assert.Contains(t, reported, "Published template to "+s3URI)
	assert.Contains(t, reported, httpsURL)
}

func TestS3URIFromPackageURL(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{name: "virtual-hosted style", in: "https://my-bucket.s3.us-east-1.amazonaws.com/templates/dev/vpc/template-abc.yaml", want: "s3://my-bucket/templates/dev/vpc/template-abc.yaml", wantOK: true},
		{name: "path style for dotted bucket", in: "https://s3.us-west-2.amazonaws.com/my.bucket.name/dev/vpc/template-abc.yaml", want: "s3://my.bucket.name/dev/vpc/template-abc.yaml", wantOK: true},
		{name: "escaped key is decoded", in: "https://b.s3.us-east-1.amazonaws.com/a%20b/t.yaml", want: "s3://b/a b/t.yaml", wantOK: true},
		{name: "empty", in: ""},
		{name: "not an s3 url", in: "https://example.com/template.yaml"},
		{name: "path style without key", in: "https://s3.us-east-1.amazonaws.com/bucket-only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := s3URIFromPackageURL(tt.in)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

// M7: apply --target <aws/stackset target> is rejected up front with a pointer to
// the stackset verbs, before packaging, confirmation or any API call.
func TestDeliverApply_StackSetTargetIsRejectedWithHint(t *testing.T) {
	stubStdinTerminal(t, false)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	octx := directApplyOctx(map[string]any{targetKey: "fanout"})
	octx.Info.ComponentSection = map[string]any{cfg.ProvisionSectionName: map[string]any{
		"targets": map[string]any{"fanout": map[string]any{"kind": kindAwsStackSet, "accounts": []any{"123456789012"}, "regions": []any{"us-east-2"}}},
	}}

	_, result, err := deliverApply(octx, client, identifiedVpcSpec())
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationStackSetTargetNotApplicable)
	assert.Nil(t, result)
	hints := strings.Join(cockroachErrors.GetAllHints(err), "\n")
	assert.Contains(t, hints, "stackset create vpc -s dev --target fanout")
	assert.Contains(t, hints, "stackset update")
}

// fakeDelivery is a registered target kind that records what apply delivered.
type fakeDelivery struct{ delivered *target.DeliverInput }

func (f *fakeDelivery) Deliver(_ context.Context, in *target.DeliverInput) error {
	f.delivered = in
	return nil
}

// H4: an external-target delivery prints what it did and asks no stack-change
// question.
func TestDeliverApply_ExternalTargetReportsDelivery(t *testing.T) {
	stubStdinTerminal(t, false)
	const kind = "test-delivery-kind"
	fake := &fakeDelivery{}
	target.Register(kind, fake)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	octx := directApplyOctx(map[string]any{})
	octx.Info.ComponentSection = map[string]any{cfg.ProvisionSectionName: map[string]any{
		"default": "gitops",
		"targets": map[string]any{"gitops": map[string]any{"kind": kind}},
	}}

	var summary map[string]any
	var err error
	out := captureStderr(t, func() { summary, _, err = deliverApply(octx, client, vpcSpec()) })
	require.NoError(t, err)
	require.NotNil(t, fake.delivered)
	assert.Equal(t, "vpc.yaml", summary["delivered_file"])
	reported := normalizeUIOutput(out)
	assert.Contains(t, reported, "Delivered vpc.yaml to "+kind+` target "gitops"`)
}

// Execution must not start when the preview itself cannot be shown: an apply of a
// failed changeset removes its stub (covered above) and a prompt error discards
// the changeset instead of leaving it behind.
func TestDeployDirect_PromptErrorDiscardsChangeSet(t *testing.T) {
	promptErr := errors.New("tty unavailable")
	stubConfirmOperation(t, false, promptErr)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	calls := expectChangeSetCreated(client, stackWithStatus(cfntypes.StackStatusUpdateComplete), completedChangeSet())
	gomock.InOrder(append(calls, client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, nil))...)

	_, err := deployDirect(&opContext{Ctx: context.Background(), Flags: map[string]any{}}, client, vpcSpec())
	require.ErrorIs(t, err, promptErr)
}

// M5: a bulk `output` run with a structured format prints ONE document keyed by
// stack, then component, instead of concatenated unlabeled documents.
func TestBulkOutput_StructuredFormatsAggregateIntoOneDocument(t *testing.T) {
	iolib.Reset()
	t.Cleanup(iolib.Reset)
	iolib.GetContext().Masker().SetEnabled(false)

	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			flags, collector, err := withBulkOutputCollector(OperationOutput, map[string]any{"format": format})
			require.NoError(t, err)
			require.NotNil(t, collector)

			nodes := []struct{ stack, component, key, value string }{
				{"dev", "basic", "Value", "one"},
				{"dev", "consumer", "Arn", "two"},
				{"prod", "basic", "Value", "three"},
			}
			for _, node := range nodes {
				client := NewMockCloudFormationClient(gomock.NewController(t))
				client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{
					StackStatus: cfntypes.StackStatusCreateComplete,
					Outputs:     []cfntypes.Output{{OutputKey: awsString(node.key), OutputValue: awsString(node.value)}},
				}}}, nil)
				octx := &opContext{Ctx: context.Background(), Flags: flags, Info: &schema.ConfigAndStacksInfo{Stack: node.stack, ComponentFromArg: node.component}}
				stdout := captureStdout(t, func() {
					_, runErr := runOutputOperation(octx, client, &stackSpec{StackName: node.stack + "-" + node.component}, map[string]any{})
					require.NoError(t, runErr)
				})
				assert.Empty(t, stdout, "nothing may be printed until the single aggregated document")
			}

			stdout := captureStdout(t, func() { require.NoError(t, collector.flush()) })
			var doc map[string]map[string]map[string]string
			if format == "json" {
				require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
			} else {
				require.NoError(t, yaml.Unmarshal([]byte(stdout), &doc), stdout)
			}
			assert.Equal(t, map[string]map[string]map[string]string{
				"dev":  {"basic": {"Value": "one"}, "consumer": {"Arn": "two"}},
				"prod": {"basic": {"Value": "three"}},
			}, doc)
		})
	}
}

// Table output prints a title above each component's table; the other formats
// stay per-component streams (no collector).
func TestWithBulkOutputCollector_FormatSelection(t *testing.T) {
	tests := []struct {
		name          string
		operation     Operation
		format        string
		wantCollector bool
	}{
		{name: "table gets titles", operation: OperationOutput, format: "", wantCollector: true},
		{name: "json aggregates", operation: OperationOutput, format: "json", wantCollector: true},
		{name: "env stays flat", operation: OperationOutput, format: "env", wantCollector: false},
		{name: "apply is untouched", operation: OperationApply, format: "json", wantCollector: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := map[string]any{"format": tt.format}
			flags, collector, err := withBulkOutputCollector(tt.operation, in)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCollector, collector != nil)
			assert.NotContains(t, in, bulkOutputCollectorKey, "the caller's flags must not be modified")
			_, installed := flags[bulkOutputCollectorKey]
			assert.Equal(t, tt.wantCollector, installed)
		})
	}
}

func TestWithBulkOutputCollector_InvalidFormatFailsBeforeAnyComponentRuns(t *testing.T) {
	_, _, err := withBulkOutputCollector(OperationOutput, map[string]any{"format": "bogus"})
	require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
}

func TestBulkOutput_TableTitlesEachComponent(t *testing.T) {
	iolib.Reset()
	t.Cleanup(iolib.Reset)
	iolib.GetContext().Masker().SetEnabled(false)
	flags, collector, err := withBulkOutputCollector(OperationOutput, map[string]any{})
	require.NoError(t, err)

	client := NewMockCloudFormationClient(gomock.NewController(t))
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{
		StackStatus: cfntypes.StackStatusCreateComplete,
		Outputs:     []cfntypes.Output{{OutputKey: awsString("Value"), OutputValue: awsString("one")}},
	}}}, nil)
	octx := &opContext{Ctx: context.Background(), Flags: flags, Info: &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "basic"}}
	stdout := captureStdout(t, func() {
		_, runErr := runOutputOperation(octx, client, &stackSpec{StackName: "s"}, map[string]any{})
		require.NoError(t, runErr)
	})
	assert.Contains(t, stdout, "basic in stack dev:")
	assert.Contains(t, stdout, "Value")
	assert.NoError(t, collector.flush(), "table output streams per component, so there is nothing to flush")
}

// A failed apply still records the stack's final status (for example
// ROLLBACK_COMPLETE) in the summary, so CI can report it next to the error.
func TestRunApply_FailedDeployRecordsFinalStatus(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	expectDeployDirectFlow(t, client, cfntypes.StackStatusRollbackComplete)

	octx := directApplyOctx(map[string]any{"auto-approve": true})
	summary, err := runApply(octx, client, vpcSpec(), map[string]any{})
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationOperationFailed)
	assert.Equal(t, string(cfntypes.StackStatusRollbackComplete), summary["final_status"])
}

// Negative path: an apply that never executed anything (declined) reports no
// final status, and neither does a preview.
func TestRunApply_DeclinedHasNoFinalStatus(t *testing.T) {
	stubConfirmOperation(t, false, nil)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	calls := expectChangeSetCreated(client, stackWithStatus(cfntypes.StackStatusUpdateComplete), completedChangeSet())
	gomock.InOrder(append(calls, client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, nil))...)

	summary, err := runApply(directApplyOctx(map[string]any{}), client, vpcSpec(), map[string]any{})
	require.ErrorIs(t, err, errUtils.ErrUserAborted)
	assert.NotContains(t, summary, "final_status")
}

// identifiedVpcSpec is vpcSpec with the Atmos component and stack recorded, as
// resolveSpecAndTemplate records them for every real operation.
func identifiedVpcSpec() *stackSpec {
	return vpcSpec().withAtmosIdentity(&schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev"})
}

// Hints name a runnable command: the recorded component and stack, with a
// placeholder only for a part that was never recorded.
func TestStackSpecCommandTarget(t *testing.T) {
	tests := []struct {
		name string
		info *schema.ConfigAndStacksInfo
		want string
	}{
		{name: "both recorded", info: &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev"}, want: "vpc -s dev"},
		{name: "stack missing", info: &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"}, want: "vpc -s <stack>"},
		{name: "component missing", info: &schema.ConfigAndStacksInfo{Stack: "dev"}, want: "<component> -s dev"},
		{name: "nothing recorded", info: nil, want: "<component> -s <stack>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, vpcSpec().withAtmosIdentity(tt.info).commandTarget())
		})
	}
}
