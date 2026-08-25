package cloudformation

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"

	"github.com/cloudposse/atmos/pkg/perf"
)

// CloudFormationClient is the narrow subset of the AWS SDK v2 CloudFormation API
// used by this component type. Phase 1 covers the core lifecycle (changeset-driven
// apply/deploy, delete, validate, describe); later phases extend this interface
// (drift, stack sets, GetTemplate/GetStackPolicy, ListStacks) without touching
// callers that only need the Phase 1 subset.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_client_test.go -package=cloudformation -source=client.go CloudFormationClient
type CloudFormationClient interface {
	CreateChangeSet(ctx context.Context, params *cloudformation.CreateChangeSetInput, optFns ...func(*cloudformation.Options)) (*cloudformation.CreateChangeSetOutput, error)
	DescribeChangeSet(ctx context.Context, params *cloudformation.DescribeChangeSetInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DescribeChangeSetOutput, error)
	ExecuteChangeSet(ctx context.Context, params *cloudformation.ExecuteChangeSetInput, optFns ...func(*cloudformation.Options)) (*cloudformation.ExecuteChangeSetOutput, error)
	DescribeStacks(ctx context.Context, params *cloudformation.DescribeStacksInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error)
	DescribeStackEvents(ctx context.Context, params *cloudformation.DescribeStackEventsInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DescribeStackEventsOutput, error)
	DeleteStack(ctx context.Context, params *cloudformation.DeleteStackInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DeleteStackOutput, error)
	ValidateTemplate(ctx context.Context, params *cloudformation.ValidateTemplateInput, optFns ...func(*cloudformation.Options)) (*cloudformation.ValidateTemplateOutput, error)
	UpdateTerminationProtection(ctx context.Context, params *cloudformation.UpdateTerminationProtectionInput, optFns ...func(*cloudformation.Options)) (*cloudformation.UpdateTerminationProtectionOutput, error)
	SetStackPolicy(ctx context.Context, params *cloudformation.SetStackPolicyInput, optFns ...func(*cloudformation.Options)) (*cloudformation.SetStackPolicyOutput, error)
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
