package organization

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/organizations/types"

	errUtils "github.com/cloudposse/atmos/errors"
	awsIdentity "github.com/cloudposse/atmos/pkg/aws/identity"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// OrganizationInfo holds the information returned by AWS Organizations DescribeOrganization.
type OrganizationInfo struct {
	ID                 string
	Arn                string
	MasterAccountID    string
	MasterAccountEmail string
}

// Getter provides an interface for retrieving AWS organization information.
// This interface enables dependency injection and testability.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_organization.go -package=organization
type Getter interface {
	// GetOrganization retrieves the AWS organization info for the current account.
	// Returns the organization ID, ARN, master account ID, and master account email.
	GetOrganization(
		ctx context.Context,
		atmosConfig *schema.AtmosConfiguration,
		authContext *schema.AWSAuthContext,
	) (*OrganizationInfo, error)
}

// defaultGetter is the production implementation that uses real AWS SDK calls.
type defaultGetter struct{}

// GetOrganization retrieves the AWS organization info using the Organizations DescribeOrganization API.
func (d *defaultGetter) GetOrganization(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	authContext *schema.AWSAuthContext,
) (*OrganizationInfo, error) {
	defer perf.Track(atmosConfig, "organization.Getter.GetOrganization")()

	log.Debug("Getting AWS organization info")

	// Load AWS config using the shared identity package.
	cfg, err := awsIdentity.LoadConfigWithAuth(ctx, "", "", 0, authContext)
	if err != nil {
		return nil, err
	}

	// Create Organizations client and describe organization.
	orgClient := organizations.NewFromConfig(cfg)
	output, err := orgClient.DescribeOrganization(ctx, &organizations.DescribeOrganizationInput{})
	if err != nil {
		// Check for the specific "not in an organization" error.
		var notInUseErr *types.AWSOrganizationsNotInUseException
		if errors.As(err, &notInUseErr) {
			return nil, fmt.Errorf("%w: the AWS account is not a member of an organization", errUtils.ErrAwsDescribeOrganization)
		}
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsDescribeOrganization, err)
	}

	info := &OrganizationInfo{}

	// Extract values from the organization output.
	org := output.Organization
	if org == nil {
		return info, nil
	}

	if org.Id != nil {
		info.ID = *org.Id
	}
	if org.Arn != nil {
		info.Arn = *org.Arn
	}
	if org.MasterAccountId != nil {
		info.MasterAccountID = *org.MasterAccountId
	}
	if org.MasterAccountEmail != nil {
		info.MasterAccountEmail = *org.MasterAccountEmail
	}

	log.Debug("Retrieved AWS organization info",
		"id", info.ID,
		"arn", info.Arn,
		"master_account_id", info.MasterAccountID,
	)

	return info, nil
}

// getter is the global instance used by functions.
// This allows test code to replace it with a mock.
var getter Getter = &defaultGetter{}

// SetGetter allows tests to inject a mock Getter.
// Returns a function to restore the original getter.
func SetGetter(g Getter) func() {
	defer perf.Track(nil, "organization.SetGetter")()

	original := getter
	getter = g
	return func() {
		getter = original
	}
}

// cachedOrganization holds the cached AWS organization info.
// The cache is per-CLI-invocation (stored in memory) to avoid repeated API calls.
type cachedOrganization struct {
	info *OrganizationInfo
	err  error
}

var (
	organizationCache   map[string]*cachedOrganization
	organizationCacheMu sync.RWMutex
)

func init() {
	organizationCache = make(map[string]*cachedOrganization)
}

// getCacheKey generates a cache key based on the auth context.
// Different auth contexts (different credentials) get different cache entries.
// Includes Profile, CredentialsFile, and ConfigFile since all three affect AWS config loading.
func getCacheKey(authContext *schema.AWSAuthContext) string {
	defer perf.Track(nil, "organization.getCacheKey")()

	if authContext == nil {
		return "default"
	}
	return fmt.Sprintf("%s:%s:%s", authContext.Profile, authContext.CredentialsFile, authContext.ConfigFile)
}

// GetOrganizationCached retrieves the AWS organization info with caching.
// Results are cached per auth context to avoid repeated API calls within the same CLI invocation.
func GetOrganizationCached(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	authContext *schema.AWSAuthContext,
) (*OrganizationInfo, error) {
	defer perf.Track(atmosConfig, "organization.GetOrganizationCached")()

	cacheKey := getCacheKey(authContext)

	// Check cache first (read lock).
	organizationCacheMu.RLock()
	if cached, ok := organizationCache[cacheKey]; ok {
		organizationCacheMu.RUnlock()
		log.Debug("Using cached AWS organization info", "cache_key", cacheKey)
		return cached.info, cached.err
	}
	organizationCacheMu.RUnlock()

	// Cache miss - acquire write lock and fetch.
	organizationCacheMu.Lock()
	defer organizationCacheMu.Unlock()

	// Double-check after acquiring write lock.
	if cached, ok := organizationCache[cacheKey]; ok {
		log.Debug("Using cached AWS organization info (double-check)", "cache_key", cacheKey)
		return cached.info, cached.err
	}

	// Fetch from AWS.
	info, err := getter.GetOrganization(ctx, atmosConfig, authContext)

	// Cache the result (including errors to avoid repeated failed calls).
	organizationCache[cacheKey] = &cachedOrganization{
		info: info,
		err:  err,
	}

	return info, err
}

// ClearOrganizationCache clears the AWS organization cache.
// This is useful in tests or when credentials change during execution.
func ClearOrganizationCache() {
	defer perf.Track(nil, "organization.ClearOrganizationCache")()

	organizationCacheMu.Lock()
	defer organizationCacheMu.Unlock()
	organizationCache = make(map[string]*cachedOrganization)
}
