package cloudformation

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFindS3Targets(t *testing.T) {
	provisionSection := map[string]any{
		"targets": map[string]any{
			"artifacts": map[string]any{"kind": "aws/s3", "bucket": "my-bucket"},
			"aws":       map[string]any{"kind": "aws/cloudformation"},
			"gitops":    map[string]any{"kind": "git"},
		},
	}

	s3Targets := findS3Targets(provisionSection)
	require.Len(t, s3Targets, 1)
	assert.Equal(t, "my-bucket", s3Targets["artifacts"]["bucket"])
}

func TestFindS3Targets_NoTargets(t *testing.T) {
	assert.Empty(t, findS3Targets(nil))
	assert.Empty(t, findS3Targets(map[string]any{}))
}

func TestS3ConfigFromTarget(t *testing.T) {
	cfg, err := s3ConfigFromTarget("artifacts", map[string]any{
		"bucket": "my-bucket",
		"prefix": "vpc/",
		"region": "us-east-2",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-bucket", cfg.Bucket)
	assert.Equal(t, "vpc/", cfg.Prefix)
	assert.Equal(t, "us-east-2", cfg.Region)
}

func TestS3ConfigFromTarget_MissingBucket(t *testing.T) {
	_, err := s3ConfigFromTarget("artifacts", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
}

func TestResolvePackagingTarget_DirectS3Selection(t *testing.T) {
	selected := &target.SelectedTarget{
		Kind:   "aws/s3",
		Name:   "artifacts",
		Config: map[string]any{"bucket": "my-bucket"},
	}
	cfg, err := resolvePackagingTarget(nil, selected)
	require.NoError(t, err)
	assert.Equal(t, "my-bucket", cfg.Bucket)
}

func TestResolvePackagingTarget_NoS3TargetDeclared(t *testing.T) {
	selected := &target.SelectedTarget{Kind: "git", Name: "gitops"}
	_, err := resolvePackagingTarget(nil, selected)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
}

func TestResolvePackagingTarget_ImplicitSingleS3Target(t *testing.T) {
	provisionSection := map[string]any{
		"targets": map[string]any{
			"artifacts": map[string]any{"kind": "aws/s3", "bucket": "my-bucket"},
			"gitops":    map[string]any{"kind": "git"},
		},
	}
	selected := &target.SelectedTarget{Kind: "git", Name: "gitops"}
	cfg, err := resolvePackagingTarget(provisionSection, selected)
	require.NoError(t, err)
	assert.Equal(t, "my-bucket", cfg.Bucket)
}

func TestResolvePackagingTarget_AmbiguousWithoutDisambiguator(t *testing.T) {
	provisionSection := map[string]any{
		"targets": map[string]any{
			"bucket-a": map[string]any{"kind": "aws/s3", "bucket": "bucket-a"},
			"bucket-b": map[string]any{"kind": "aws/s3", "bucket": "bucket-b"},
			"gitops":   map[string]any{"kind": "git"},
		},
	}
	selected := &target.SelectedTarget{Kind: "git", Name: "gitops", Config: map[string]any{"kind": "git"}}
	_, err := resolvePackagingTarget(provisionSection, selected)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
}

// authManagerFor must return nil when no auth manager is configured, or when
// the configured value isn't an auth.AuthManager.
func TestAuthManagerFor_NilWhenNoManager(t *testing.T) {
	assert.Nil(t, authManagerFor(&schema.ConfigAndStacksInfo{}))
	assert.Nil(t, authManagerFor(&schema.ConfigAndStacksInfo{AuthManager: "not-a-manager"}))
}

// authManagerFor must surface a configured auth.AuthManager as the target
// registry's identity-environment provider.
func TestAuthManagerFor_ReturnsConfiguredManager(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockManager := authtypes.NewMockAuthManager(ctrl)

	provider := authManagerFor(&schema.ConfigAndStacksInfo{AuthManager: mockManager})
	require.NotNil(t, provider)
	assert.Equal(t, target.IdentityEnvironmentProvider(mockManager), provider)
}

// deliverToExternalTarget must route through the generic target registry
// (populating template_bytes in the summary first) and propagate the
// registered git provisioner's own validation error when no repository is
// configured for the target — proving delivery actually reaches
// target.Deliver rather than short-circuiting.
func TestDeliverToExternalTarget_GitTargetNotConfigured(t *testing.T) {
	octx := &opContext{
		Ctx:         context.Background(),
		AtmosConfig: &schema.AtmosConfiguration{},
		Info:        &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev"},
	}
	selected := &target.SelectedTarget{Kind: "git", Name: "gitops"}
	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}
	summary := map[string]any{}

	err := deliverToExternalTarget(octx, selected, spec, summary)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGitRepositoryNotFound)
	assert.Equal(t, len(spec.TemplateBody), summary["template_bytes"])
}

// deployDirect must skip ExecuteChangeSet and streamStackEvents when the
// changeset is a no-op — a no-op apply must not attempt to execute anything.
func TestDeployDirect_NoOp(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)
	client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateChangeSetOutput{}, nil)
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
		Status:       cfntypes.ChangeSetStatusFailed,
		StatusReason: awsString("The submitted information didn't contain changes."),
	}, nil)
	// No ExecuteChangeSet/DescribeStackEvents expectations: a call to either
	// fails the test via gomock's unexpected-call panic.

	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}
	result, err := deployDirect(context.Background(), client, spec)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.NoOp)
}

// expectDeployDirectFlow sets up the common gomock expectation chain a
// direct-deploy exercises: stack-doesn't-exist check, changeset create +
// compute, execute, and event-stream polling to the given final status.
// Shared by TestDeployDirect_Success/_FailedFinalStatus and
// TestDeliverApply_DirectDeployKind so the two callers of deployDirect don't
// each hand-roll the same six-call sequence.
func expectDeployDirectFlow(client *MockCloudFormationClient, finalStatus cfntypes.StackStatus) {
	gomock.InOrder(
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil),
		client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateChangeSetOutput{}, nil),
		client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
			Status: cfntypes.ChangeSetStatusCreateComplete,
		}, nil),
		client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.ExecuteChangeSetOutput{}, nil),
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
			Stacks: []cfntypes.Stack{{StackStatus: finalStatus}},
		}, nil),
	)
}

// deployDirect's happy path: create, execute, and stream events to a
// successful terminal status.
func TestDeployDirect_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	expectDeployDirectFlow(client, cfntypes.StackStatusCreateComplete)

	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}
	result, err := deployDirect(context.Background(), client, spec)
	require.NoError(t, err)
	assert.False(t, result.NoOp)
}

// deployDirect must return an error wrapping ErrAwsCloudFormationChangeSetFailed
// when the stack converges to a failed terminal status.
func TestDeployDirect_FailedFinalStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	expectDeployDirectFlow(client, cfntypes.StackStatusCreateFailed)

	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}
	_, err := deployDirect(context.Background(), client, spec)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
}

// deliverApply must route to deployDirect (the implicit default target) when
// the component declares no provision section.
func TestDeliverApply_DirectDeployKind(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	expectDeployDirectFlow(client, cfntypes.StackStatusCreateComplete)

	octx := &opContext{
		Ctx:         context.Background(),
		AtmosConfig: &schema.AtmosConfiguration{},
		Info:        &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}},
		Flags:       map[string]any{},
	}
	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}

	summary, result, err := deliverApply(octx, client, spec)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.NoOp)
	assert.Equal(t, "default", summary[targetKey])
}

// deliverApply must be publish-only when a `kind: aws/s3` target is directly
// selected: upload the template and stop — no CloudFormationClient call.
func TestDeliverApply_DirectS3Selection_PublishOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl) // no expectations: any call fails the test.

	mockBackend := artifact.NewMockBackend(ctrl)
	mockBackend.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	stubNewS3Backend(t, mockBackend, nil)

	octx := &opContext{
		Ctx:         context.Background(),
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
			ComponentSection: map[string]any{
				cfg.ProvisionSectionName: map[string]any{
					"targets": map[string]any{
						"artifacts": map[string]any{"kind": "aws/s3", "bucket": "my-bucket"},
					},
				},
			},
		},
		Flags: map[string]any{targetKey: "artifacts"},
	}
	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}

	summary, result, err := deliverApply(octx, client, spec)
	require.NoError(t, err)
	assert.Nil(t, result, "publish-only delivery must not run a deploy")
	assert.Equal(t, "artifacts", summary[targetKey])
	assert.Equal(t, "s3://my-bucket/dev/vpc/template-", summary["package_url"].(string)[:len("s3://my-bucket/dev/vpc/template-")])
	assert.NotEmpty(t, summary["package_sha256"])
}

// deliverApply must package a large template through the resolved aws/s3
// target before handing it to the external (non-direct-deploy) target
// registry — asserted here against a "git" target with no repository
// configured, so the packaging step's summary fields are observable even
// though final delivery fails on the git provisioner's own validation.
func TestDeliverApply_ExternalTarget_PackagesLargeTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl) // no expectations: git delivery never touches CloudFormation.

	mockBackend := artifact.NewMockBackend(ctrl)
	mockBackend.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	stubNewS3Backend(t, mockBackend, nil)

	largeTemplate := "AWSTemplateFormatVersion: '2010-09-09'\n" + strings.Repeat("a", templateInlineSizeLimit+1)

	octx := &opContext{
		Ctx:         context.Background(),
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
			ComponentSection: map[string]any{
				cfg.ProvisionSectionName: map[string]any{
					"default": "gitops",
					"targets": map[string]any{
						"gitops":    map[string]any{"kind": "git"},
						"artifacts": map[string]any{"kind": "aws/s3", "bucket": "my-bucket"},
					},
				},
			},
		},
		Flags: map[string]any{},
	}
	spec := &stackSpec{StackName: "vpc", TemplateBody: largeTemplate}

	summary, result, err := deliverApply(octx, client, spec)
	require.Error(t, err, "the git target has no repository configured")
	assert.ErrorIs(t, err, errUtils.ErrGitRepositoryNotFound)
	assert.Nil(t, result)
	assert.Equal(t, "gitops", summary[targetKey])
	assert.NotEmpty(t, summary["package_url"], "packaging must run before the failed delivery attempt")
	assert.NotEmpty(t, summary["package_sha256"])
}

// deliverApply must propagate a target-selection error (e.g. an explicit
// --target naming an undeclared target) without attempting any delivery.
func TestDeliverApply_SelectTargetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl) // no expectations: selection fails before any client use.

	octx := &opContext{
		Ctx:         context.Background(),
		AtmosConfig: &schema.AtmosConfiguration{},
		Info:        &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}},
		Flags:       map[string]any{targetKey: "nonexistent"},
	}
	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}

	_, result, err := deliverApply(octx, client, spec)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProvisionTargetNotFound)
	assert.Nil(t, result)
}

// deliverApply must propagate a packaging-target resolution error (e.g. an
// external kind with no `kind: aws/s3` target declared to package through).
func TestDeliverApply_PackagingTargetResolutionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl) // no expectations: resolution fails before any client use.

	octx := &opContext{
		Ctx:         context.Background(),
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			ComponentSection: map[string]any{
				cfg.ProvisionSectionName: map[string]any{
					"targets": map[string]any{
						"gitops": map[string]any{"kind": "git"},
					},
				},
			},
		},
		Flags: map[string]any{targetKey: "gitops"},
	}
	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}

	_, result, err := deliverApply(octx, client, spec)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
	assert.Nil(t, result)
}

func TestResolvePackagingTarget_DisambiguatedWithPackagingField(t *testing.T) {
	provisionSection := map[string]any{
		"targets": map[string]any{
			"bucket-a": map[string]any{"kind": "aws/s3", "bucket": "bucket-a"},
			"bucket-b": map[string]any{"kind": "aws/s3", "bucket": "bucket-b"},
			"gitops":   map[string]any{"kind": "git", "packaging": "bucket-b"},
		},
	}
	selected := &target.SelectedTarget{Kind: "git", Name: "gitops", Config: map[string]any{"kind": "git", "packaging": "bucket-b"}}
	cfg, err := resolvePackagingTarget(provisionSection, selected)
	require.NoError(t, err)
	assert.Equal(t, "bucket-b", cfg.Bucket)
}
