package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	awsAmbientProviderName = "aws-ambient"
	awsAmbientKind         = "aws/ambient"
	logKeyAmbientIdentity  = "identity"
)

// awsAmbientIdentity implements AWS ambient identity.
// It resolves credentials from the AWS SDK's default credential provider chain
// (environment variables, shared config, IRSA web identity, EC2 instance profile / IMDS).
// Unlike other AWS identities, it does NOT clear credential environment variables
// or disable IMDS in PrepareEnvironment.
type awsAmbientIdentity struct {
	name   string
	config *schema.Identity
	realm  string // Credential isolation realm set by auth manager.
}

// NewAWSAmbientIdentity creates a new AWS ambient identity.
func NewAWSAmbientIdentity(name string, config *schema.Identity) (types.Identity, error) {
	defer perf.Track(nil, "aws.NewAWSAmbientIdentity")()

	if config == nil {
		return nil, fmt.Errorf("%w: identity %q has nil config", errUtils.ErrInvalidAuthConfig, name)
	}
	if config.Kind != awsAmbientKind {
		return nil, fmt.Errorf("%w: invalid identity kind for aws/ambient: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}
	if config.Via != nil {
		return nil, fmt.Errorf("%w: aws/ambient identity %q must not define via", errUtils.ErrInvalidIdentityConfig, name)
	}

	return &awsAmbientIdentity{
		name:   name,
		config: config,
	}, nil
}

// Kind returns the identity kind.
func (i *awsAmbientIdentity) Kind() string {
	return awsAmbientKind
}

// SetRealm sets the credential isolation realm for this identity.
// AWS ambient identities store the realm but don't use it for file operations.
func (i *awsAmbientIdentity) SetRealm(realm string) {
	i.realm = realm
}

// GetProviderName returns the provider name for this identity.
// AWS ambient identities are standalone and always return "aws-ambient".
func (i *awsAmbientIdentity) GetProviderName() (string, error) {
	return awsAmbientProviderName, nil
}

// Authenticate resolves credentials from the AWS SDK's default credential provider chain.
// This uses config.LoadDefaultConfig directly (without clearing env vars or disabling IMDS),
// allowing IRSA, instance profiles, environment variables, and shared config to participate.
// The resolved credentials are returned as AWSCredentials so chained identities
// (e.g., aws/assume-role) can use them as base credentials.
func (i *awsAmbientIdentity) Authenticate(ctx context.Context, _ types.ICredentials) (types.ICredentials, error) {
	defer perf.Track(nil, "aws.awsAmbientIdentity.Authenticate")()

	log.Debug("Resolving ambient AWS credentials from default provider chain", logKeyAmbientIdentity, i.name)

	region := i.resolveRegion()

	// Load AWS config using the default credential chain.
	// Intentionally NOT using awsCloud.LoadIsolatedAWSConfig or WithIsolatedAWSEnv
	// because we want IRSA, IMDS, env vars, and shared config to all participate.
	var optFns []func(*config.LoadOptions) error
	if region != "" {
		optFns = append(optFns, config.WithRegion(region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load default AWS config for ambient identity %q: %w",
			errUtils.ErrLoadAWSConfig, i.name, err)
	}

	// Fall back to SDK-resolved region when no explicit region is configured.
	resolvedRegion := region
	if resolvedRegion == "" {
		resolvedRegion = cfg.Region
	}

	// Retrieve credentials from the resolved config.
	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to retrieve ambient AWS credentials for identity %q: %w",
			errUtils.ErrAuthenticationFailed, i.name, err)
	}

	log.Debug("Ambient AWS credentials resolved successfully",
		logKeyAmbientIdentity, i.name,
		"source", creds.Source,
		"has_session_token", creds.SessionToken != "",
	)

	// Build AWSCredentials from the resolved credentials.
	awsCreds := &types.AWSCredentials{
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.SessionToken,
		Region:          resolvedRegion,
	}

	// Set expiration if available.
	if creds.CanExpire {
		awsCreds.Expiration = creds.Expires.Format("2006-01-02T15:04:05Z")
	}

	return awsCreds, nil
}

// Validate validates the identity configuration.
// AWS ambient identities have no required fields.
func (i *awsAmbientIdentity) Validate() error {
	return nil
}

// Environment returns environment variables for this identity.
// AWS ambient identities do not set any environment variables.
func (i *awsAmbientIdentity) Environment() (map[string]string, error) {
	return map[string]string{}, nil
}

// Paths returns credential files/directories used by this identity.
// AWS ambient identities do not use any credential files.
func (i *awsAmbientIdentity) Paths() ([]types.Path, error) {
	return []types.Path{}, nil
}

// PrepareEnvironment prepares environment variables for external processes.
// Unlike other AWS identities, this does NOT:
//   - Clear credential environment variables (AWS_ACCESS_KEY_ID, etc.)
//   - Set AWS_EC2_METADATA_DISABLED
//   - Override AWS_SHARED_CREDENTIALS_FILE or AWS_CONFIG_FILE
//   - Set AWS_PROFILE
//
// It only optionally sets AWS_REGION and AWS_DEFAULT_REGION if configured.
func (i *awsAmbientIdentity) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "aws.awsAmbientIdentity.PrepareEnvironment")()

	// Create a copy to avoid mutating the input.
	result := make(map[string]string, len(environ))
	for k, v := range environ {
		result[k] = v
	}

	// Only set region if explicitly configured.
	region := i.resolveRegion()
	if region != "" {
		result["AWS_REGION"] = region
		result["AWS_DEFAULT_REGION"] = region
	}

	log.Debug("AWS ambient identity prepared environment (passthrough)", logKeyAmbientIdentity, i.name, "region", region)
	return result, nil
}

// PostAuthenticate is a no-op for AWS ambient identities.
// Ambient identities do not write credential files.
func (i *awsAmbientIdentity) PostAuthenticate(_ context.Context, _ *types.PostAuthenticateParams) error {
	return nil
}

// Logout is a no-op for AWS ambient identities.
func (i *awsAmbientIdentity) Logout(_ context.Context) error {
	return nil
}

// CredentialsExist checks if ambient AWS credentials are available.
// This attempts to load the default AWS config and retrieve credentials.
func (i *awsAmbientIdentity) CredentialsExist() (bool, error) {
	defer perf.Track(nil, "aws.awsAmbientIdentity.CredentialsExist")()

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return false, nil
	}

	_, err = cfg.Credentials.Retrieve(ctx)
	return err == nil, nil
}

// LoadCredentials resolves credentials from the default AWS credential chain.
func (i *awsAmbientIdentity) LoadCredentials(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "aws.awsAmbientIdentity.LoadCredentials")()

	return i.Authenticate(ctx, nil)
}

// resolveRegion returns the configured region from the principal, or empty string.
func (i *awsAmbientIdentity) resolveRegion() string {
	if i.config.Principal != nil {
		if r, ok := i.config.Principal["region"].(string); ok && r != "" {
			return r
		}
	}
	return ""
}

// IsStandaloneAWSAmbientChain checks if the authentication chain represents a standalone AWS ambient identity.
func IsStandaloneAWSAmbientChain(chain []string, identities map[string]schema.Identity) bool {
	if len(chain) != 1 {
		return false
	}

	identityName := chain[0]
	if identity, exists := identities[identityName]; exists {
		return identity.Kind == awsAmbientKind
	}

	return false
}

// AuthenticateStandaloneAWSAmbient handles authentication for standalone AWS ambient identities.
func AuthenticateStandaloneAWSAmbient(ctx context.Context, identityName string, identities map[string]types.Identity) (types.ICredentials, error) {
	defer perf.Track(nil, "aws.AuthenticateStandaloneAWSAmbient")()

	log.Debug("Authenticating AWS ambient identity directly", logKeyAmbientIdentity, identityName)

	identity, exists := identities[identityName]
	if !exists {
		return nil, fmt.Errorf("%w: AWS ambient identity %q not found", errUtils.ErrInvalidAuthConfig, identityName)
	}

	// AWS ambient identities resolve credentials from the default chain.
	credentials, err := identity.Authenticate(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: AWS ambient identity %q authentication failed: %w",
			errUtils.ErrAuthenticationFailed, identityName, err)
	}

	log.Debug("AWS ambient identity authenticated successfully", logKeyAmbientIdentity, identityName)
	return credentials, nil
}
