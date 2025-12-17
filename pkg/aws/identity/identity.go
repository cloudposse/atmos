package identity

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// CallerIdentity holds the information returned by AWS STS GetCallerIdentity.
type CallerIdentity struct {
	Account string
	Arn     string
	UserID  string
	Region  string // The AWS region from the loaded config.
}

// Getter provides an interface for retrieving AWS caller identity information.
// This interface enables dependency injection and testability.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_getter_test.go -package=identity
type Getter interface {
	// GetCallerIdentity retrieves the AWS caller identity for the current credentials.
	// Returns the account ID, ARN, and user ID of the calling identity.
	GetCallerIdentity(
		ctx context.Context,
		atmosConfig *schema.AtmosConfiguration,
		authContext *schema.AWSAuthContext,
	) (*CallerIdentity, error)
}

// defaultGetter is the production implementation that uses real AWS SDK calls.
type defaultGetter struct{}

// GetCallerIdentity retrieves the AWS caller identity using the STS GetCallerIdentity API.
func (d *defaultGetter) GetCallerIdentity(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	authContext *schema.AWSAuthContext,
) (*CallerIdentity, error) {
	defer perf.Track(atmosConfig, "identity.Getter.GetCallerIdentity")()

	log.Debug("Getting AWS caller identity")

	// Use the exported function to get caller identity.
	result, err := GetCallerIdentity(ctx, "", "", 0, authContext)
	if err != nil {
		return nil, err
	}

	identity := &CallerIdentity{
		Account: result.Account,
		Arn:     result.Arn,
		UserID:  result.UserID,
		Region:  result.Region,
	}

	log.Debug("Retrieved AWS caller identity",
		"account", identity.Account,
		"arn", identity.Arn,
		"user_id", identity.UserID,
		"region", identity.Region,
	)

	return identity, nil
}

// getter is the global instance used by functions.
// This allows test code to replace it with a mock.
var getter Getter = &defaultGetter{}

// SetGetter allows tests to inject a mock Getter.
// Returns a function to restore the original getter.
func SetGetter(g Getter) func() {
	defer perf.Track(nil, "identity.SetGetter")()

	original := getter
	getter = g
	return func() {
		getter = original
	}
}

// cachedIdentity holds the cached AWS caller identity.
// The cache is per-CLI-invocation (stored in memory) to avoid repeated STS calls.
type cachedIdentity struct {
	identity *CallerIdentity
	err      error
}

var (
	identityCache   map[string]*cachedIdentity
	identityCacheMu sync.RWMutex
)

func init() {
	identityCache = make(map[string]*cachedIdentity)
}

// getCacheKey generates a cache key based on the auth context.
// Different auth contexts (different credentials) get different cache entries.
// Includes Profile, CredentialsFile, and ConfigFile since all three affect AWS config loading.
func getCacheKey(authContext *schema.AWSAuthContext) string {
	defer perf.Track(nil, "identity.getCacheKey")()

	if authContext == nil {
		return "default"
	}
	return fmt.Sprintf("%s:%s:%s", authContext.Profile, authContext.CredentialsFile, authContext.ConfigFile)
}

// GetCallerIdentityCached retrieves the AWS caller identity with caching.
// Results are cached per auth context to avoid repeated STS calls within the same CLI invocation.
func GetCallerIdentityCached(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	authContext *schema.AWSAuthContext,
) (*CallerIdentity, error) {
	defer perf.Track(atmosConfig, "identity.GetCallerIdentityCached")()

	cacheKey := getCacheKey(authContext)

	// Check cache first (read lock).
	identityCacheMu.RLock()
	if cached, ok := identityCache[cacheKey]; ok {
		identityCacheMu.RUnlock()
		log.Debug("Using cached AWS caller identity", "cache_key", cacheKey)
		return cached.identity, cached.err
	}
	identityCacheMu.RUnlock()

	// Cache miss - acquire write lock and fetch.
	identityCacheMu.Lock()
	defer identityCacheMu.Unlock()

	// Double-check after acquiring write lock.
	if cached, ok := identityCache[cacheKey]; ok {
		log.Debug("Using cached AWS caller identity (double-check)", "cache_key", cacheKey)
		return cached.identity, cached.err
	}

	// Fetch from AWS.
	identity, err := getter.GetCallerIdentity(ctx, atmosConfig, authContext)

	// Cache the result (including errors to avoid repeated failed calls).
	identityCache[cacheKey] = &cachedIdentity{
		identity: identity,
		err:      err,
	}

	return identity, err
}

// ClearIdentityCache clears the AWS identity cache.
// This is useful in tests or when credentials change during execution.
func ClearIdentityCache() {
	defer perf.Track(nil, "identity.ClearIdentityCache")()

	identityCacheMu.Lock()
	defer identityCacheMu.Unlock()
	identityCache = make(map[string]*cachedIdentity)
}

// GetCallerIdentity retrieves AWS caller identity using STS GetCallerIdentity API.
// Returns account ID, ARN, user ID, and region.
// This function keeps AWS SDK STS imports contained within this package.
// For caching, use GetCallerIdentityCached instead.
func GetCallerIdentity(
	ctx context.Context,
	region string,
	roleArn string,
	assumeRoleDuration time.Duration,
	authContext *schema.AWSAuthContext,
) (*CallerIdentity, error) {
	defer perf.Track(nil, "identity.GetCallerIdentity")()

	// Load AWS config.
	cfg, err := LoadConfigWithAuth(ctx, region, roleArn, assumeRoleDuration, authContext)
	if err != nil {
		return nil, err
	}

	// Create STS client and get caller identity.
	stsClient := sts.NewFromConfig(cfg)
	output, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsGetCallerIdentity, err)
	}

	result := &CallerIdentity{
		Region: cfg.Region,
	}

	// Extract values from pointers.
	if output.Account != nil {
		result.Account = *output.Account
	}
	if output.Arn != nil {
		result.Arn = *output.Arn
	}
	if output.UserId != nil {
		result.UserID = *output.UserId
	}

	return result, nil
}

// LoadConfigWithAuth loads AWS config, preferring auth context if available.
//
// When authContext is provided, it uses the Atmos-managed credentials files and profile.
// Otherwise, it falls back to standard AWS SDK credential resolution.
//
// Standard AWS SDK credential resolution order:
//
//	Environment variables:
//	  AWS_ACCESS_KEY_ID
//	  AWS_SECRET_ACCESS_KEY
//	  AWS_SESSION_TOKEN (optional, for temporary credentials)
//
//	Shared credentials file:
//	  Typically at ~/.aws/credentials
//	  Controlled by:
//	    AWS_PROFILE (defaults to default)
//	    AWS_SHARED_CREDENTIALS_FILE
//
//	Shared config file:
//	  Typically at ~/.aws/config
//	  Also supports named profiles and region settings
//
//	Amazon EC2 Instance Metadata Service (IMDS):
//	  If running on EC2 or ECS
//	  Uses IAM roles attached to the instance/task
//
//	Web Identity Token credentials:
//	  When AWS_WEB_IDENTITY_TOKEN_FILE and AWS_ROLE_ARN are set (e.g., in EKS)
//
//	SSO credentials (if configured)
//
//	Custom credential sources:
//	  Provided programmatically using config.WithCredentialsProvider(...)
func LoadConfigWithAuth(
	ctx context.Context,
	region string,
	roleArn string,
	assumeRoleDuration time.Duration,
	authContext *schema.AWSAuthContext,
) (aws.Config, error) {
	defer perf.Track(nil, "identity.LoadConfigWithAuth")()

	var cfgOpts []func(*config.LoadOptions) error

	// If auth context is provided, use Atmos-managed credentials.
	if authContext != nil {
		log.Debug("Using Atmos auth context for AWS SDK",
			"profile", authContext.Profile,
			"credentials", authContext.CredentialsFile,
			"config", authContext.ConfigFile,
		)

		// Set custom credential and config file paths.
		// This overrides the default ~/.aws/credentials and ~/.aws/config.
		cfgOpts = append(cfgOpts,
			config.WithSharedCredentialsFiles([]string{authContext.CredentialsFile}),
			config.WithSharedConfigFiles([]string{authContext.ConfigFile}),
			config.WithSharedConfigProfile(authContext.Profile),
		)

		// Use region from auth context if not explicitly provided.
		if region == "" && authContext.Region != "" {
			region = authContext.Region
		}
	} else {
		log.Debug("Using standard AWS SDK credential resolution (no auth context provided)")
	}

	// Set region if provided.
	if region != "" {
		log.Debug("Using explicit region", "region", region)
		cfgOpts = append(cfgOpts, config.WithRegion(region))
	}

	// Load base config.
	log.Debug("Loading AWS SDK config", "num_options", len(cfgOpts))
	baseCfg, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		log.Debug("Failed to load AWS config", "error", err)
		return aws.Config{}, fmt.Errorf("%w: %w", errUtils.ErrLoadAWSConfig, err)
	}
	log.Debug("Successfully loaded AWS SDK config", "region", baseCfg.Region)

	// Conditionally assume role if specified.
	if roleArn != "" {
		log.Debug("Assuming role", "ARN", roleArn)
		stsClient := sts.NewFromConfig(baseCfg)

		creds := stscreds.NewAssumeRoleProvider(stsClient, roleArn, func(o *stscreds.AssumeRoleOptions) {
			o.Duration = assumeRoleDuration
		})

		cfgOpts = append(cfgOpts, config.WithCredentialsProvider(aws.NewCredentialsCache(creds)))

		// Reload full config with assumed role credentials.
		return config.LoadDefaultConfig(ctx, cfgOpts...)
	}

	return baseCfg, nil
}

// LoadConfig loads AWS config using standard AWS SDK credential resolution.
// This is a wrapper around LoadConfigWithAuth for convenience.
// For code that needs Atmos auth support, use LoadConfigWithAuth instead.
func LoadConfig(ctx context.Context, region string, roleArn string, assumeRoleDuration time.Duration) (aws.Config, error) {
	defer perf.Track(nil, "identity.LoadConfig")()

	return LoadConfigWithAuth(ctx, region, roleArn, assumeRoleDuration, nil)
}
