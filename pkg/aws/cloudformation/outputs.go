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
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"

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
		if isStackNotFoundError(err) {
			return nil, fmt.Errorf("%w: stack %q: %w", errUtils.ErrAwsCloudFormationStackNotFound, stackName, err)
		}
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationAPICallFailed, err)
	}
	if len(out.Stacks) == 0 {
		return nil, fmt.Errorf("%w: stack %q", errUtils.ErrAwsCloudFormationStackNotFound, stackName)
	}
	if err := CheckStackDeployed(stackName, out.Stacks[0].StackStatus); err != nil {
		return nil, err
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

// isStackNotFoundError reports whether err is CloudFormation's "does not exist" ValidationError,
// returned by DescribeStacks for a named stack that was never deployed (as opposed to an
// account-wide DescribeStacks call, which instead returns an empty Stacks slice). Requires the
// smithy.APIError's ErrorCode to be "ValidationError" before checking the message — CloudFormation
// uses this generic code for several unrelated validation failures, so the message substring alone
// is not a reliable classifier. Loosely mirrors pkg/component/aws/cloudformation's own
// isStackNotFoundError classifier — kept as a small local copy rather than a shared import to avoid
// coupling this intentionally narrow leaf package (see the package doc comment) to the full
// CloudFormation component implementation.
func isStackNotFoundError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.ErrorCode() == "ValidationError" && strings.Contains(apiErr.ErrorMessage(), "does not exist")
}

// notDeployedStatuses are the stack statuses that carry no deployed resources
// (and therefore no meaningful Outputs): a stack that only holds a preview
// changeset (REVIEW_IN_PROGRESS, left behind by diff/plan on a never-deployed
// component), a failed or rolled-back create, or one that is deleted or being
// deleted.
var notDeployedStatuses = map[cfntypes.StackStatus]bool{
	cfntypes.StackStatusReviewInProgress:   true,
	cfntypes.StackStatusRollbackInProgress: true,
	cfntypes.StackStatusRollbackComplete:   true,
	cfntypes.StackStatusRollbackFailed:     true,
	cfntypes.StackStatusCreateFailed:       true,
	cfntypes.StackStatusDeleteInProgress:   true,
	cfntypes.StackStatusDeleteComplete:     true,
	cfntypes.StackStatusDeleteFailed:       true,
}

// CheckStackDeployed returns ErrAwsCloudFormationStackNotDeployed when status
// is one that carries no deployed resources, so callers never mistake a stub
// stack's empty Outputs for real values.
func CheckStackDeployed(stackName string, status cfntypes.StackStatus) error {
	defer perf.Track(nil, "cloudformation.CheckStackDeployed")()

	if !notDeployedStatuses[status] {
		return nil
	}
	return errUtils.Build(fmt.Errorf("%w: %q has status %s", errUtils.ErrAwsCloudFormationStackNotDeployed, stackName, status)).
		WithHint("Deploy the stack with `atmos aws cloudformation apply`. A stack stuck in a failed-create state (ROLLBACK_COMPLETE, CREATE_FAILED) must be deleted first with `atmos aws cloudformation delete`.").
		Err()
}

// LookupOutput returns the value of one Output key, or
// ErrAwsCloudFormationOutputNotFound naming the key and listing the keys the
// stack does have, so a misspelled key fails loudly instead of resolving to
// null.
func LookupOutput(outputs map[string]any, stackName, key string) (any, error) {
	defer perf.Track(nil, "cloudformation.LookupOutput")()

	if value, ok := outputs[key]; ok {
		return value, nil
	}
	return nil, errUtils.Build(fmt.Errorf("%w: %q in stack %q (available: %s)", errUtils.ErrAwsCloudFormationOutputNotFound, key, stackName, availableOutputKeys(outputs))).
		WithHint("Output keys are case-sensitive. Check the `Outputs` section of the stack's template.").
		Err()
}

// availableOutputKeys renders the sorted output keys for an error message.
func availableOutputKeys(outputs map[string]any) string {
	if len(outputs) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(outputs))
	for k := range outputs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
