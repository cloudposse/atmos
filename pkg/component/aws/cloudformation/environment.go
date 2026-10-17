package cloudformation

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/cloudposse/atmos/pkg/aws/identity"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
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

	awsCfg, err := identity.LoadConfigWithAuth(ctx, region, "", 0, awsAuthContextFrom(info))
	if err != nil {
		return awsCfg, err
	}
	warnIgnoredEnvRegion(info, awsCfg.Region)
	return awsCfg, nil
}

// envRegionVars are the component `env` entries a user might expect to select
// the CloudFormation region.
var envRegionVars = []string{"AWS_REGION", "AWS_DEFAULT_REGION"}

// warnIgnoredEnvRegion warns when the component's `env` sets AWS_REGION or
// AWS_DEFAULT_REGION to a region other than the one the SDK client resolved.
// The client is built in-process from the active identity, never from the
// component `env` (see buildAWSConfig), so by the documented precedence the
// env value is ignored. Say so instead of silently deploying to a different
// region than the user wrote.
func warnIgnoredEnvRegion(info *schema.ConfigAndStacksInfo, resolvedRegion string) {
	if info == nil || resolvedRegion == "" {
		return
	}
	for _, name := range envRegionVars {
		value, ok := componentEnvValue(info, name)
		if !ok || value == "" || value == resolvedRegion {
			continue
		}
		ui.Warningf("Component env sets %s=%s, which is ignored: CloudFormation uses region %s. Set settings.aws_cloudformation.region to override the region.", name, value, resolvedRegion)
	}
}

// componentEnvValue looks up one variable in the component's resolved `env`
// section, falling back to the raw component section's `env` map.
func componentEnvValue(info *schema.ConfigAndStacksInfo, name string) (string, bool) {
	if value, ok := info.ComponentEnvSection[name]; ok {
		return fmt.Sprint(value), true
	}
	if env, ok := info.ComponentSection[cfg.EnvSectionName].(map[string]any); ok {
		if value, ok := env[name]; ok {
			return fmt.Sprint(value), true
		}
	}
	return "", false
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
