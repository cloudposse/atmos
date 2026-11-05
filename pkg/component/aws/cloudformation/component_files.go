package cloudformation

import (
	"context"

	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

// prepareComponentFiles resolves the component directory and provisions its JIT source, if any.
// S3 sources authenticate lazily: the resolver below runs only when the downloader actually
// needs credentials, so local templates and other source protocols never trigger authentication.
func prepareComponentFiles(ctx context.Context, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error) {
	path, err := resolveComponentPath(atmosConfig, info)
	if err != nil {
		return "", err
	}
	ctx = downloader.WithAWSAuthResolver(ctx, func(context.Context) (*schema.AWSAuthContext, error) {
		return resolveSourceAWSAuth(atmosConfig, info)
	})
	path, _, err = provisionAndResolveComponentPath(ctx, provisioner.OutputWriters{}, atmosConfig, info, config.CloudFormationComponentType, path)
	if err != nil {
		return "", err
	}
	return path, nil
}

// resolveSourceAWSAuth runs only when the S3 downloader needs credentials. Local
// templates, other source protocols and a warm source cache never authenticate.
func resolveSourceAWSAuth(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.AWSAuthContext, error) {
	defer perf.Track(atmosConfig, "cloudformation.resolveSourceAWSAuth")()
	if info.DryRun || info.AuthDisabled || authdeferred.AuthDisabled(atmosConfig.AuthManager) {
		return nil, nil
	}
	if info.AuthContext != nil {
		return info.AuthContext.AWS, nil
	}
	identity := config.NormalizeIdentityValue(info.Identity)
	if identity == config.IdentityFlagDisabledValue {
		return nil, nil
	}
	if identity == config.IdentityFlagSelectValue {
		identity = ""
	}
	resolved, err := authdeferred.Credentials(atmosConfig, info, identity).Resolve()
	if err != nil {
		return nil, err
	}
	info.AuthManager, info.AuthContext = resolved.AuthManager, resolved.AuthContext
	if info.AuthContext == nil {
		return nil, nil
	}
	return info.AuthContext.AWS, nil
}
