package s3

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/store"
)

// AuthResolver resolves an Atmos identity name to AWS credentials configuration. It is the AWS
// slice of store.AuthContextResolver, which satisfies it.
type AuthResolver interface {
	ResolveAWSAuthContext(ctx context.Context, identityName string) (*store.AWSAuthConfig, error)
}

// ClientConfig describes how to build an S3 client. Both the generic [Store] and the legacy CI
// adapter build clients through [NewClient] so credential, identity, and endpoint handling live in
// one place.
type ClientConfig struct {
	// Region overrides the region of the identity and the default chain.
	Region string

	// Identity is the Atmos identity name. Empty uses the default AWS credential chain.
	Identity string

	// Resolver turns Identity into credentials configuration.
	Resolver AuthResolver

	// EndpointURL overrides the S3 endpoint (for example an emulator). It wins over the endpoint of
	// a resolved identity.
	EndpointURL string

	// PathStyle selects path-style addressing. Nil means path-style whenever an endpoint override
	// applies, which is what emulators such as Floci require.
	PathStyle *bool

	// HTTPClient replaces the SDK HTTP client, for example to trust a private CA. The SDK honors
	// AWS_CA_BUNDLE when nil.
	HTTPClient aws.HTTPClient

	// Legacy keeps the pre-artifact CI behavior: when an identity is configured but no resolver was
	// injected, ambient credentials are used, and the SDK's default checksum behavior is kept. Only
	// the legacy CI adapter sets it. The generic store never does, so an unresolvable identity
	// fails closed.
	Legacy bool
}

// NewClient builds an S3 client from cfg. With an identity it resolves credentials through the
// resolver; without one it uses the default credential chain. See [ClientConfig.Legacy] for the
// one deliberate difference between the generic and legacy paths.
func NewClient(ctx context.Context, cfg *ClientConfig) (*s3.Client, error) {
	defer perf.Track(nil, "blob.s3.NewClient")()

	if cfg.Identity == "" {
		return newDefaultClient(ctx, cfg, nil)
	}
	if cfg.Resolver == nil {
		if cfg.Legacy {
			return newDefaultClient(ctx, cfg, nil)
		}
		return nil, identityError(cfg.Identity, errUtils.ErrBlobIdentityUnresolved)
	}
	auth, err := cfg.Resolver.ResolveAWSAuthContext(ctx, cfg.Identity)
	if err != nil {
		return nil, identityError(cfg.Identity, err)
	}
	if auth == nil && !cfg.Legacy {
		return nil, identityError(cfg.Identity, errUtils.ErrBlobIdentityUnresolved)
	}
	loadOpts := BuildAuthConfigOpts(cfg.Region, auth)
	awsCfg, err := config.LoadDefaultConfig(ctx, withHTTPClient(loadOpts, cfg.HTTPClient)...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAWSConfigLoadFailed, err)
	}
	return s3.NewFromConfig(awsCfg, clientOptFns(cfg, auth)...), nil
}

// identityError reports an unresolvable identity. It wraps both the configuration-load sentinel the
// legacy callers match and the blob identity sentinel the generic path matches.
func identityError(identity string, cause error) error {
	return fmt.Errorf("%w: %w: failed to resolve AWS auth context for identity %q: %w",
		errUtils.ErrAWSConfigLoadFailed, errUtils.ErrBlobIdentityUnresolved, identity, cause)
}

// newDefaultClient builds a client from the default credential chain.
func newDefaultClient(ctx context.Context, cfg *ClientConfig, auth *store.AWSAuthConfig) (*s3.Client, error) {
	var loadOpts []func(*config.LoadOptions) error
	if cfg.Region != "" {
		loadOpts = append(loadOpts, config.WithRegion(cfg.Region))
	}
	awsCfg, err := config.LoadDefaultConfig(ctx, withHTTPClient(loadOpts, cfg.HTTPClient)...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAWSConfigLoadFailed, err)
	}
	return s3.NewFromConfig(awsCfg, clientOptFns(cfg, auth)...), nil
}

func withHTTPClient(opts []func(*config.LoadOptions) error, client aws.HTTPClient) []func(*config.LoadOptions) error {
	if client == nil {
		return opts
	}
	return append(opts, config.WithHTTPClient(client))
}

// clientOptFns assembles the S3 client options for cfg and the resolved identity context.
func clientOptFns(cfg *ClientConfig, auth *store.AWSAuthConfig) []func(*s3.Options) {
	var optFns []func(*s3.Options)
	endpoint := cfg.EndpointURL
	if endpoint == "" && auth != nil {
		endpoint = auth.EndpointURL
	}
	if endpoint != "" {
		pathStyle := true
		if cfg.PathStyle != nil {
			pathStyle = *cfg.PathStyle
		}
		optFns = append(optFns, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = pathStyle
		})
	} else if cfg.PathStyle != nil {
		pathStyle := *cfg.PathStyle
		optFns = append(optFns, func(o *s3.Options) { o.UsePathStyle = pathStyle })
	}
	if !cfg.Legacy {
		// Integrity is established by artifact digests. Skipping optional request checksums keeps
		// multipart completion and emulator endpoints simple.
		optFns = append(optFns, func(o *s3.Options) {
			o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
			o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
		})
	}
	return optFns
}

// BuildClientOptFns returns the S3 client options an identity's auth context requires: an endpoint
// override (for example an emulator) with path-style addressing. Without it an identity scoped to an
// emulator would still route S3 calls to the real AWS endpoint.
func BuildClientOptFns(auth *store.AWSAuthConfig) []func(*s3.Options) {
	defer perf.Track(nil, "blob.s3.BuildClientOptFns")()

	return clientOptFns(&ClientConfig{Legacy: true}, auth)
}

// BuildAuthConfigOpts returns the AWS SDK config options for a resolved auth context. The
// store-level region wins over the identity's region.
func BuildAuthConfigOpts(region string, auth *store.AWSAuthConfig) []func(*config.LoadOptions) error {
	defer perf.Track(nil, "blob.s3.BuildAuthConfigOpts")()

	var cfgOpts []func(*config.LoadOptions) error
	if auth == nil {
		return cfgOpts
	}
	if auth.CredentialsFile != "" {
		cfgOpts = append(cfgOpts, config.WithSharedCredentialsFiles([]string{auth.CredentialsFile}))
	}
	if auth.ConfigFile != "" {
		cfgOpts = append(cfgOpts, config.WithSharedConfigFiles([]string{auth.ConfigFile}))
	}
	if auth.Profile != "" {
		cfgOpts = append(cfgOpts, config.WithSharedConfigProfile(auth.Profile))
	}
	if region == "" {
		region = auth.Region
	}
	if region != "" {
		cfgOpts = append(cfgOpts, config.WithRegion(region))
	}
	return cfgOpts
}
