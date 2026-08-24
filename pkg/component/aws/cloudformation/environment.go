package cloudformation

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/cloudposse/atmos/pkg/aws/identity"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// buildAWSConfig resolves an aws.Config for the active identity, in-process.
//
// Unlike the shell-out component types (helm's applyAuthEnvironment), this never
// mutates the process environment via os.Setenv: the SDK client is constructed
// directly from the resolved aws.Config, safe under concurrent bulk (--all/
// --affected) execution. This mirrors the established pattern used by
// !terraform.state/!terraform.output (pkg/aws/identity.LoadConfigWithAuth) rather
// than PR #2536's subprocess-oriented auth hook, which does not apply to an
// SDK-native component.
//
// When info.AuthContext.AWS is set (an AWS identity is active), its
// EndpointURL — when present — is honored automatically by LoadConfigWithAuth,
// which is how a Floci-emulated CloudFormation endpoint reaches this client with
// no CFN-specific emulator code.
func buildAWSConfig(ctx context.Context, info *schema.ConfigAndStacksInfo, region string) (aws.Config, error) {
	defer perf.Track(nil, "cloudformation.buildAWSConfig")()

	var awsAuthContext *schema.AWSAuthContext
	if info.AuthContext != nil {
		awsAuthContext = info.AuthContext.AWS
	}

	return identity.LoadConfigWithAuth(ctx, region, "", 0, awsAuthContext)
}
