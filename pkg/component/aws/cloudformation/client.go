package cloudformation

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"

	"github.com/cloudposse/atmos/pkg/perf"
)

// CloudFormationClient is the subset of the AWS SDK v2 CloudFormation API used
// by this component type. Phase 1 covered the core lifecycle (changeset-driven
// apply/deploy, delete, validate, describe); Phase 2 added explicit changeset
// management, drift detection, template/policy retrieval, and account-wide
// listing. Phase 3 adds stack sets and nested-stack observability (tree/logs).
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_client_test.go -package=cloudformation -source=client.go CloudFormationClient
type CloudFormationClient interface {
	CreateChangeSet(ctx context.Context, params *cloudformation.CreateChangeSetInput, optFns ...func(*cloudformation.Options)) (*cloudformation.CreateChangeSetOutput, error)
	DescribeChangeSet(ctx context.Context, params *cloudformation.DescribeChangeSetInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DescribeChangeSetOutput, error)
	ExecuteChangeSet(ctx context.Context, params *cloudformation.ExecuteChangeSetInput, optFns ...func(*cloudformation.Options)) (*cloudformation.ExecuteChangeSetOutput, error)
	ListChangeSets(ctx context.Context, params *cloudformation.ListChangeSetsInput, optFns ...func(*cloudformation.Options)) (*cloudformation.ListChangeSetsOutput, error)
	DeleteChangeSet(ctx context.Context, params *cloudformation.DeleteChangeSetInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DeleteChangeSetOutput, error)
	DescribeStacks(ctx context.Context, params *cloudformation.DescribeStacksInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error)
	DescribeStackEvents(ctx context.Context, params *cloudformation.DescribeStackEventsInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DescribeStackEventsOutput, error)
	DeleteStack(ctx context.Context, params *cloudformation.DeleteStackInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DeleteStackOutput, error)
	ValidateTemplate(ctx context.Context, params *cloudformation.ValidateTemplateInput, optFns ...func(*cloudformation.Options)) (*cloudformation.ValidateTemplateOutput, error)
	UpdateTerminationProtection(ctx context.Context, params *cloudformation.UpdateTerminationProtectionInput, optFns ...func(*cloudformation.Options)) (*cloudformation.UpdateTerminationProtectionOutput, error)
	SetStackPolicy(ctx context.Context, params *cloudformation.SetStackPolicyInput, optFns ...func(*cloudformation.Options)) (*cloudformation.SetStackPolicyOutput, error)
	GetTemplate(ctx context.Context, params *cloudformation.GetTemplateInput, optFns ...func(*cloudformation.Options)) (*cloudformation.GetTemplateOutput, error)
	GetStackPolicy(ctx context.Context, params *cloudformation.GetStackPolicyInput, optFns ...func(*cloudformation.Options)) (*cloudformation.GetStackPolicyOutput, error)
	ListStacks(ctx context.Context, params *cloudformation.ListStacksInput, optFns ...func(*cloudformation.Options)) (*cloudformation.ListStacksOutput, error)
	DetectStackDrift(ctx context.Context, params *cloudformation.DetectStackDriftInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DetectStackDriftOutput, error)
	DescribeStackDriftDetectionStatus(ctx context.Context, params *cloudformation.DescribeStackDriftDetectionStatusInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DescribeStackDriftDetectionStatusOutput, error)
	DescribeStackResourceDrifts(ctx context.Context, params *cloudformation.DescribeStackResourceDriftsInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DescribeStackResourceDriftsOutput, error)
	CreateStackSet(ctx context.Context, params *cloudformation.CreateStackSetInput, optFns ...func(*cloudformation.Options)) (*cloudformation.CreateStackSetOutput, error)
	UpdateStackSet(ctx context.Context, params *cloudformation.UpdateStackSetInput, optFns ...func(*cloudformation.Options)) (*cloudformation.UpdateStackSetOutput, error)
	DeleteStackSet(ctx context.Context, params *cloudformation.DeleteStackSetInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DeleteStackSetOutput, error)
	CreateStackInstances(ctx context.Context, params *cloudformation.CreateStackInstancesInput, optFns ...func(*cloudformation.Options)) (*cloudformation.CreateStackInstancesOutput, error)
	DeleteStackInstances(ctx context.Context, params *cloudformation.DeleteStackInstancesInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DeleteStackInstancesOutput, error)
	ListStackInstances(ctx context.Context, params *cloudformation.ListStackInstancesInput, optFns ...func(*cloudformation.Options)) (*cloudformation.ListStackInstancesOutput, error)
	DescribeStackSetOperation(ctx context.Context, params *cloudformation.DescribeStackSetOperationInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DescribeStackSetOperationOutput, error)
	ListStackResources(ctx context.Context, params *cloudformation.ListStackResourcesInput, optFns ...func(*cloudformation.Options)) (*cloudformation.ListStackResourcesOutput, error)
}

// newClient constructs the real AWS SDK v2 CloudFormation client from a resolved
// aws.Config (built in-process via environment.go, honoring the active identity's
// credentials). EndpointURL overrides the service endpoint (e.g. a Floci-emulated
// CloudFormation endpoint) when set — see environment.go's resolveEndpointURL.
func newClient(cfg aws.Config, endpointURL string) CloudFormationClient { //nolint:gocritic // aws.Config-by-value matches cloudformation.NewFromConfig's own signature.
	defer perf.Track(nil, "cloudformation.newClient")()

	var optFns []func(*cloudformation.Options)
	if endpointURL != "" {
		optFns = append(optFns, func(o *cloudformation.Options) { o.BaseEndpoint = aws.String(endpointURL) })
	}

	return cloudformation.NewFromConfig(cfg, optFns...)
}
