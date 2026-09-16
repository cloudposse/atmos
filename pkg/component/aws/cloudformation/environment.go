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
func buildAWSConfig(ctx context.Context, info *schema.ConfigAndStacksInfo, region string) (aws.Config, error) {
	defer perf.Track(nil, "cloudformation.buildAWSConfig")()

	return identity.LoadConfigWithAuth(ctx, region, "", 0, awsAuthContextFrom(info))
}

// awsAuthContextFrom extracts the active identity's AWSAuthContext from info, or
// nil when no AWS identity is active.
func awsAuthContextFrom(info *schema.ConfigAndStacksInfo) *schema.AWSAuthContext {
	if info == nil || info.AuthContext == nil {
		return nil
	}
	return info.AuthContext.AWS
}

// resolveEndpointURL returns the active identity's AWS service endpoint
// override, or "" when none is set (real AWS).
//
// SDK v2 endpoint overrides are per-service client options, not part of
// aws.Config, so they can't be set once on the aws.Config returned by
// buildAWSConfig — they must be applied at client construction time (see
// newClient). This mirrors pkg/provisioner/backend/s3.go's s3ClientOptions and
// pkg/store/providers/aws_ssm_param_store.go, which apply the same
// AuthContext.AWS.EndpointURL fallback for their own service clients. Without
// this, an emulator-backed identity (aws/emulator) silently targets real AWS
// instead of the local emulator endpoint.
func resolveEndpointURL(info *schema.ConfigAndStacksInfo) string {
	if authCtx := awsAuthContextFrom(info); authCtx != nil {
		return authCtx.EndpointURL
	}
	return ""
}
