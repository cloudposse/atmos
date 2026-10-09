package cloudformation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	crerrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestSourceInferenceRejectsAmbiguousProvisionedResults requires an explicit path for empty, missing,
// nested, or multiple-file source results.
func TestSourceInferenceRejectsAmbiguousProvisionedResults(t *testing.T) {
	for _, tt := range []struct {
		name      string
		files     []string
		directory bool
		missing   bool
	}{
		{name: "empty"},
		{name: "metadata only", files: []string{".metadata"}},
		{name: "multiple files", files: []string{"first.yaml", "second.yaml"}},
		{name: "nested directory", directory: true},
		{name: "missing directory", missing: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("Resources: {}"), 0o600))
			}
			if tt.directory {
				require.NoError(t, os.Mkdir(filepath.Join(dir, "templates"), 0o700))
			}
			if tt.missing {
				dir = filepath.Join(dir, "missing")
			}
			stubProvisionAndResolveComponentPath(t, dir, nil)
			info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"stack_name": "source-test", "source": map[string]any{"uri": "https://example.test/source"}}}
			spec, err := resolveSpecAndTemplate(context.Background(), &schema.AtmosConfiguration{}, info, OperationRender)
			require.ErrorIs(t, err, errUtils.ErrMissingAwsCloudFormationTemplate)
			assert.Nil(t, spec)
			if tt.directory || len(tt.files) > 1 {
				assert.Contains(t, crerrors.GetAllHints(err), "Set path explicitly when the provisioned source contains a directory or multiple files.")
			}
		})
	}
}

// TestSourceInferenceIgnoresProvisioningMetadata allows hidden provisioning metadata alongside the
// single inferred template.
func TestSourceInferenceIgnoresProvisioningMetadata(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".metadata"), []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "template.yaml"), []byte("Resources: {}"), 0o600))
	name, err := inferSourceTemplatePath(dir)
	require.NoError(t, err)
	assert.Equal(t, "template.yaml", name)
}

// TestNamedChangesetProtectionFailureIsReportedAfterCompletion preserves the successful deployment
// status while reporting a subsequent protection failure.
func TestNamedChangesetProtectionFailureIsReportedAfterCompletion(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	denied := errors.New("termination protection denied")
	enabled := true
	gomock.InOrder(
		client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{Status: cfntypes.ChangeSetStatusCreateComplete}, nil),
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil),
		client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.ExecuteChangeSetOutput{}, nil),
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{StackEvents: []cfntypes.StackEvent{{EventId: awsString("completed"), ResourceStatus: cfntypes.ResourceStatusCreateComplete}}}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateComplete}}}, nil),
		client.EXPECT().UpdateTerminationProtection(gomock.Any(), &cloudformation.UpdateTerminationProtectionInput{StackName: awsString("protected"), EnableTerminationProtection: &enabled}).Return(nil, denied),
	)
	summary, err := runChangesetExecute(context.Background(), client, &stackSpec{StackName: "protected", TerminationProtection: true}, "reviewed", map[string]any{})
	require.ErrorIs(t, err, denied)
	assert.Equal(t, string(cfntypes.StackStatusCreateComplete), summary["final_status"])
}

// TestDriftDescribeCountsOnlyModifiedAndDeletedAcrossPages excludes clean and unchecked resources from
// the count passed to CI.
func TestDriftDescribeCountsOnlyModifiedAndDeletedAcrossPages(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	first := []cfntypes.StackResourceDrift{{StackResourceDriftStatus: cfntypes.StackResourceDriftStatusInSync}, {StackResourceDriftStatus: cfntypes.StackResourceDriftStatusModified}}
	second := []cfntypes.StackResourceDrift{{StackResourceDriftStatus: cfntypes.StackResourceDriftStatusDeleted}, {StackResourceDriftStatus: cfntypes.StackResourceDriftStatusNotChecked}}
	gomock.InOrder(
		client.EXPECT().DescribeStackResourceDrifts(gomock.Any(), &cloudformation.DescribeStackResourceDriftsInput{StackName: awsString("mixed")}).Return(&cloudformation.DescribeStackResourceDriftsOutput{StackResourceDrifts: first, NextToken: awsString("page-two")}, nil),
		client.EXPECT().DescribeStackResourceDrifts(gomock.Any(), &cloudformation.DescribeStackResourceDriftsInput{StackName: awsString("mixed"), NextToken: awsString("page-two")}).Return(&cloudformation.DescribeStackResourceDriftsOutput{StackResourceDrifts: second}, nil),
	)
	summary, err := runDriftDescribe(context.Background(), client, "mixed", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, int32(2), summary["drifted_resource_count"])
	assert.Len(t, summary["drifts"], 4)
	result := &schema.CloudFormationCIResult{}
	populateCloudFormationCIResultFromSummary(result, summary)
	assert.Equal(t, 2, result.DriftedCount)
}

// TestStackSetMalformedTargetStopsBeforeMutation rejects invalid account and region types before API
// writes in normal execution and during dry-run validation.
func TestStackSetMalformedTargetStopsBeforeMutation(t *testing.T) {
	for _, operation := range []Operation{OperationStackSetCreate, OperationStackSetUpdate} {
		for _, field := range []string{"accounts", "regions"} {
			t.Run(string(operation)+"/"+field, func(t *testing.T) {
				target := map[string]any{"kind": "aws/stackset", "accounts": []any{"012345678901"}, "regions": []any{"us-east-2"}}
				target[field] = []any{123}
				section := map[string]any{"stack_name": "malformed", "template": "Resources: {}", "provision": map[string]any{"targets": map[string]any{"test": target}}}
				client := NewMockCloudFormationClient(gomock.NewController(t))
				_, err := operationHandlers[operation](&opContext{Ctx: context.Background(), AtmosConfig: &schema.AtmosConfiguration{}, Info: &schema.ConfigAndStacksInfo{ComponentSection: section}}, client, &stackSpec{StackName: "malformed", TemplateBody: "Resources: {}"}, map[string]any{})
				require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
				assert.Contains(t, err.Error(), "provision.targets.test."+field)
				installExecutorSeamStubs(t, executorSeamStubs{processStacks: func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
					info.ComponentIsEnabled = true
					info.ComponentSection = section
					return info, nil
				}})
				err = executeSingle(&component.ExecutionContext{}, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{DryRun: true}, operation)
				require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
				assert.Contains(t, err.Error(), "provision.targets.test."+field)
			})
		}
	}
}

// TestOutputMaskingFollowsYAMLAliases traces aliased NoEcho references while retaining public values
// and the original output map.
func TestOutputMaskingFollowsYAMLAliases(t *testing.T) {
	body := `Parameters:
  Password: {Type: String, NoEcho: true}
Outputs:
  Original: {Value: &password !Ref Password}
  Aliased: {Value: *password}
  Safe: {Value: public-value}
`
	raw := map[string]any{"Original": "alias-secret-canary", "Aliased": "alias-secret-canary", "Safe": "public-value"}
	presented, err := maskStackOutputs(body, nil, raw)
	require.NoError(t, err)
	assert.Equal(t, "<MASKED>", presented["Original"])
	assert.Equal(t, "<MASKED>", presented["Aliased"])
	assert.Equal(t, "public-value", presented["Safe"])
	assert.Equal(t, "alias-secret-canary", raw["Aliased"], "masking must not alter dependency values")
}
