// Package cloudformation provides a minimal, direct AWS SDK v2 read of a
// deployed CloudFormation stack's Outputs, for consumers outside
// pkg/component/aws/cloudformation (which already owns the full CFN component
// implementation but cannot be imported back into internal/exec — it imports
// internal/exec itself). Kept intentionally narrow: a single DescribeStacks
// call, mirroring pkg/aws/identity's and pkg/aws/organization's shape as a
// small, provider-specific leaf package.
package cloudformation

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/aws/identity"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// cloudFormationAPI is the subset of the AWS SDK v2 CloudFormation client used
// by this package. A seam for testing GetOutputs without real AWS credentials
// or network access.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_outputs_test.go -package=cloudformation
type cloudFormationAPI interface {
	DescribeStacks(ctx context.Context, params *cloudformation.DescribeStacksInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error)
}

// Seams for testing.
var (
	loadAWSConfig           = identity.LoadConfigWithAuth
	newCloudFormationClient = defaultNewCloudFormationClient
)

// defaultNewCloudFormationClient constructs the real AWS SDK v2 CloudFormation
// client from a resolved aws.Config. The endpointURL parameter overrides the
// service endpoint (e.g. a Floci-emulated CloudFormation endpoint) when set,
// matching pkg/component/aws/cloudformation's own client construction
// (client.go's newClient).
func defaultNewCloudFormationClient(cfg aws.Config, endpointURL string) cloudFormationAPI { //nolint:gocritic // aws.Config-by-value matches cloudformation.NewFromConfig's own signature.
	var optFns []func(*cloudformation.Options)
	if endpointURL != "" {
		optFns = append(optFns, func(o *cloudformation.Options) { o.BaseEndpoint = aws.String(endpointURL) })
	}
	return cloudformation.NewFromConfig(cfg, optFns...)
}

// GetOutputs fetches a deployed CloudFormation stack's Outputs, keyed by
// Output name. AuthContext's EndpointURL (when set) is honored so this also
// reaches a Floci-emulated CloudFormation endpoint, matching
// pkg/component/aws/cloudformation's own client construction.
func GetOutputs(ctx context.Context, region, stackName string, authContext *schema.AWSAuthContext) (map[string]any, error) {
	defer perf.Track(nil, "cloudformation.GetOutputs")()

	awsCfg, err := loadAWSConfig(ctx, region, "", 0, authContext)
	if err != nil {
		return nil, err
	}

	endpointURL := ""
	if authContext != nil {
		endpointURL = authContext.EndpointURL
	}

	client := newCloudFormationClient(awsCfg, endpointURL)
	out, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: aws.String(stackName)})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationChangeSetFailed, err)
	}
	if len(out.Stacks) == 0 {
		return nil, fmt.Errorf("%w: stack %q", errUtils.ErrAwsCloudFormationChangeSetFailed, stackName)
	}

	outputs := make(map[string]any, len(out.Stacks[0].Outputs))
	for _, o := range out.Stacks[0].Outputs {
		if o.OutputKey == nil {
			continue
		}
		var value any
		if o.OutputValue != nil {
			value = *o.OutputValue
		}
		outputs[*o.OutputKey] = value
	}
	return outputs, nil
}
