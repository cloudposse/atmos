package exec

import (
	"context"
	"fmt"
	"sync"

	awsUtils "github.com/cloudposse/atmos/internal/aws_utils"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AWSCallerIdentity holds the information returned by AWS STS GetCallerIdentity.
type AWSCallerIdentity struct {
	Account string
	Arn     string
	UserID  string
	Region  string // The AWS region from the loaded config.
}

// AWSGetter provides an interface for retrieving AWS caller identity information.
// This interface enables dependency injection and testability.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_aws_getter_test.go -package=exec
type AWSGetter interface {
	// GetCallerIdentity retrieves the AWS caller identity for the current credentials.
	// Returns the account ID, ARN, and user ID of the calling identity.
	GetCallerIdentity(
		ctx context.Context,
		atmosConfig *schema.AtmosConfiguration,
		authContext *schema.AWSAuthContext,
	) (*AWSCallerIdentity, error)
}

// defaultAWSGetter is the production implementation that uses real AWS SDK calls.
type defaultAWSGetter struct{}

// GetCallerIdentity retrieves the AWS caller identity using the STS GetCallerIdentity API.
func (d *defaultAWSGetter) GetCallerIdentity(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	authContext *schema.AWSAuthContext,
) (*AWSCallerIdentity, error) {
	defer perf.Track(atmosConfig, "exec.AWSGetter.GetCallerIdentity")()

	log.Debug("Getting AWS caller identity")

	// Use the aws_utils helper to get caller identity (keeps AWS SDK imports in aws_utils).
	result, err := awsUtils.GetAWSCallerIdentity(ctx, "", "", 0, authContext)
	if err != nil {
		return nil, err // Error already wrapped by aws_utils.
	}

	identity := &AWSCallerIdentity{
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

// awsGetter is the global instance used by YAML functions.
// This allows test code to replace it with a mock.
var awsGetter AWSGetter = &defaultAWSGetter{}

// SetAWSGetter allows tests to inject a mock AWSGetter.
// Returns a function to restore the original getter.
func SetAWSGetter(getter AWSGetter) func() {
	defer perf.Track(nil, "exec.SetAWSGetter")()

	original := awsGetter
	awsGetter = getter
	return func() {
		awsGetter = original
	}
}

// cachedAWSIdentity holds the cached AWS caller identity.
// The cache is per-CLI-invocation (stored in memory) to avoid repeated STS calls.
type cachedAWSIdentity struct {
	identity *AWSCallerIdentity
	err      error
}

var (
	awsIdentityCache   map[string]*cachedAWSIdentity
	awsIdentityCacheMu sync.RWMutex
)

func init() {
	awsIdentityCache = make(map[string]*cachedAWSIdentity)
}

// getCacheKey generates a cache key based on the auth context.
// Different auth contexts (different credentials) get different cache entries.
// Includes Profile, CredentialsFile, and ConfigFile since all three affect AWS config loading.
func getCacheKey(authContext *schema.AWSAuthContext) string {
	if authContext == nil {
		return "default"
	}
	return fmt.Sprintf("%s:%s:%s", authContext.Profile, authContext.CredentialsFile, authContext.ConfigFile)
}

// getAWSCallerIdentityCached retrieves the AWS caller identity with caching.
// Results are cached per auth context to avoid repeated STS calls within the same CLI invocation.
func getAWSCallerIdentityCached(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	authContext *schema.AWSAuthContext,
) (*AWSCallerIdentity, error) {
	defer perf.Track(atmosConfig, "exec.getAWSCallerIdentityCached")()

	cacheKey := getCacheKey(authContext)

	// Check cache first (read lock).
	awsIdentityCacheMu.RLock()
	if cached, ok := awsIdentityCache[cacheKey]; ok {
		awsIdentityCacheMu.RUnlock()
		log.Debug("Using cached AWS caller identity", "cache_key", cacheKey)
		return cached.identity, cached.err
	}
	awsIdentityCacheMu.RUnlock()

	// Cache miss - acquire write lock and fetch.
	awsIdentityCacheMu.Lock()
	defer awsIdentityCacheMu.Unlock()

	// Double-check after acquiring write lock.
	if cached, ok := awsIdentityCache[cacheKey]; ok {
		log.Debug("Using cached AWS caller identity (double-check)", "cache_key", cacheKey)
		return cached.identity, cached.err
	}

	// Fetch from AWS.
	identity, err := awsGetter.GetCallerIdentity(ctx, atmosConfig, authContext)

	// Cache the result (including errors to avoid repeated failed calls).
	awsIdentityCache[cacheKey] = &cachedAWSIdentity{
		identity: identity,
		err:      err,
	}

	return identity, err
}

// ClearAWSIdentityCache clears the AWS identity cache.
// This is useful in tests or when credentials change during execution.
func ClearAWSIdentityCache() {
	defer perf.Track(nil, "exec.ClearAWSIdentityCache")()

	awsIdentityCacheMu.Lock()
	defer awsIdentityCacheMu.Unlock()
	awsIdentityCache = make(map[string]*cachedAWSIdentity)
}
