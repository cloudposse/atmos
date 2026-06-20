package sopsauth

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/getsops/sops/v3/kms"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/store"
)

// AWSKMS resolves the identity's AWS credentials and wraps them as a getsops KMS credentials
// provider. The KMS endpoint itself is resolved by getsops from the standard AWS endpoint
// environment variables; only credentials are injected here.
func (b *resolverBuilder) AWSKMS(ctx context.Context, identity string) (KMSApplier, error) {
	defer perf.Track(nil, "sopsauth.AWSKMS")()

	authContext, err := b.resolver.ResolveAWSAuthContext(ctx, identity)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve AWS auth context for SOPS identity %q: %w", identity, err)
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsAuthConfigOpts(authContext)...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for SOPS identity %q: %w", identity, err)
	}
	return kms.NewCredentialsProvider(cfg.Credentials), nil
}

// awsAuthConfigOpts mirrors the store AWS credential-option building (see SSMStore.buildAuthConfigOpts).
func awsAuthConfigOpts(authContext *store.AWSAuthConfig) []func(*awsconfig.LoadOptions) error {
	var opts []func(*awsconfig.LoadOptions) error
	if authContext.CredentialsFile != "" {
		opts = append(opts, awsconfig.WithSharedCredentialsFiles([]string{authContext.CredentialsFile}))
	}
	if authContext.ConfigFile != "" {
		opts = append(opts, awsconfig.WithSharedConfigFiles([]string{authContext.ConfigFile}))
	}
	if authContext.Profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(authContext.Profile))
	}
	if authContext.Region != "" {
		opts = append(opts, awsconfig.WithRegion(authContext.Region))
	}
	return opts
}
