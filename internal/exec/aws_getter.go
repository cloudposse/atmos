package exec

import (
	"context"

	awsIdentity "github.com/cloudposse/atmos/pkg/aws/identity"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AWSCallerIdentity holds the information returned by AWS STS GetCallerIdentity.
// This is a type alias that delegates to pkg/aws/identity.CallerIdentity.
type AWSCallerIdentity = awsIdentity.CallerIdentity

// AWSGetter provides an interface for retrieving AWS caller identity information.
// This interface enables dependency injection and testability.
// This is a type alias that delegates to pkg/aws/identity.Getter.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_aws_getter_test.go -package=exec
type AWSGetter = awsIdentity.Getter

// SetAWSGetter allows tests to inject a mock AWSGetter.
// Returns a function to restore the original getter.
func SetAWSGetter(getter AWSGetter) func() {
	defer perf.Track(nil, "exec.SetAWSGetter")()

	return awsIdentity.SetGetter(getter)
}

// getAWSCallerIdentityCached retrieves the AWS caller identity with caching.
// Results are cached per auth context to avoid repeated STS calls within the same CLI invocation.
func getAWSCallerIdentityCached(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	authContext *schema.AWSAuthContext,
) (*AWSCallerIdentity, error) {
	defer perf.Track(atmosConfig, "exec.getAWSCallerIdentityCached")()

	return awsIdentity.GetCallerIdentityCached(ctx, atmosConfig, authContext)
}

// ClearAWSIdentityCache clears the AWS identity cache.
// This is useful in tests or when credentials change during execution.
func ClearAWSIdentityCache() {
	defer perf.Track(nil, "exec.ClearAWSIdentityCache")()

	awsIdentity.ClearIdentityCache()
}
