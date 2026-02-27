package organization

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// withMockOrgClient replaces the AWS client factory functions for testing defaultGetter.
// It returns a cleanup function to restore the originals.
func withMockOrgClient(t *testing.T, mockClient organizationsAPI, configErr error) func() {
	t.Helper()

	oldLoadConfig := loadAWSConfig
	oldNewClient := newOrgClient

	loadAWSConfig = func(
		_ context.Context, _, _ string, _ time.Duration, _ *schema.AWSAuthContext,
	) (aws.Config, error) {
		if configErr != nil {
			return aws.Config{}, configErr
		}
		return aws.Config{}, nil
	}

	newOrgClient = func(_ aws.Config) organizationsAPI { return mockClient }

	return func() {
		loadAWSConfig = oldLoadConfig
		newOrgClient = oldNewClient
	}
}

func TestGetOrganizationCached_Success(t *testing.T) {
	ClearOrganizationCache()

	ctrl := gomock.NewController(t)
	mock := NewMockGetter(ctrl)

	expectedInfo := &OrganizationInfo{
		ID:                 "o-abc123",
		Arn:                "arn:aws:organizations::111111111111:organization/o-abc123",
		MasterAccountID:    "111111111111",
		MasterAccountEmail: "master@example.com",
	}

	mock.EXPECT().
		GetOrganization(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(expectedInfo, nil).
		Times(1)

	restore := SetGetter(mock)
	defer restore()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	info, err := GetOrganizationCached(ctx, atmosConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, "o-abc123", info.ID)
	assert.Equal(t, "arn:aws:organizations::111111111111:organization/o-abc123", info.Arn)
	assert.Equal(t, "111111111111", info.MasterAccountID)
	assert.Equal(t, "master@example.com", info.MasterAccountEmail)
}

func TestGetOrganizationCached_CacheBehavior(t *testing.T) {
	ClearOrganizationCache()

	ctrl := gomock.NewController(t)
	mock := NewMockGetter(ctrl)

	expectedInfo := &OrganizationInfo{
		ID:              "o-cached",
		Arn:             "arn:aws:organizations::222222222222:organization/o-cached",
		MasterAccountID: "222222222222",
	}

	// Expect exactly 3 calls: first call, different auth context, after cache clear.
	mock.EXPECT().
		GetOrganization(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(expectedInfo, nil).
		Times(3)

	restore := SetGetter(mock)
	defer restore()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	// First call should hit the mock.
	info1, err := GetOrganizationCached(ctx, atmosConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, "o-cached", info1.ID)

	// Second call with same auth context should use cache (no additional mock call).
	info2, err := GetOrganizationCached(ctx, atmosConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, "o-cached", info2.ID)

	// Call with different auth context should call getter again.
	differentAuth := &schema.AWSAuthContext{
		Profile:         "different-profile",
		CredentialsFile: "/different/path",
	}
	info3, err := GetOrganizationCached(ctx, atmosConfig, differentAuth)
	require.NoError(t, err)
	assert.Equal(t, "o-cached", info3.ID)

	// Clear cache and verify next call hits mock.
	ClearOrganizationCache()
	info4, err := GetOrganizationCached(ctx, atmosConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, "o-cached", info4.ID)
}

func TestGetOrganizationCached_ErrorCaching(t *testing.T) {
	ClearOrganizationCache()

	ctrl := gomock.NewController(t)
	mock := NewMockGetter(ctrl)

	expectedErr := errors.New("mock organization error")

	// Expect exactly 1 call; second call should use cached error.
	mock.EXPECT().
		GetOrganization(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, expectedErr).
		Times(1)

	restore := SetGetter(mock)
	defer restore()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	// First call should return error and cache it.
	_, err := GetOrganizationCached(ctx, atmosConfig, nil)
	require.Error(t, err)

	// Second call should return cached error (no additional mock call).
	_, err = GetOrganizationCached(ctx, atmosConfig, nil)
	require.Error(t, err)
}

func TestGetOrganizationCached_Concurrent(t *testing.T) {
	ClearOrganizationCache()

	ctrl := gomock.NewController(t)
	mock := NewMockGetter(ctrl)

	expectedInfo := &OrganizationInfo{
		ID:              "o-concurrent",
		Arn:             "arn:aws:organizations::333333333333:organization/o-concurrent",
		MasterAccountID: "333333333333",
	}

	// Despite 50 goroutines, expect at most 1 call due to caching.
	mock.EXPECT().
		GetOrganization(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(expectedInfo, nil).
		Times(1)

	restore := SetGetter(mock)
	defer restore()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			info, err := GetOrganizationCached(ctx, atmosConfig, nil)
			assert.NoError(t, err)
			assert.Equal(t, "o-concurrent", info.ID)
		}()
	}

	wg.Wait()
}

func TestSetGetter_Restore(t *testing.T) {
	ClearOrganizationCache()

	// Save original getter.
	originalGetter := getter

	ctrl := gomock.NewController(t)
	mock := NewMockGetter(ctrl)

	restore := SetGetter(mock)

	// Verify mock is active.
	assert.Equal(t, mock, getter)

	// Restore original.
	restore()

	// Verify original is restored.
	assert.Equal(t, originalGetter, getter)
}

func TestGetCacheKey(t *testing.T) {
	tests := []struct {
		name        string
		authContext *schema.AWSAuthContext
		expected    string
	}{
		{
			name:        "nil auth context",
			authContext: nil,
			expected:    "default",
		},
		{
			name: "with profile only",
			authContext: &schema.AWSAuthContext{
				Profile: "test-profile",
			},
			expected: "test-profile::",
		},
		{
			name: "with all fields",
			authContext: &schema.AWSAuthContext{
				Profile:         "prod",
				CredentialsFile: "/creds",
				ConfigFile:      "/config",
			},
			expected: "prod:/creds:/config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCacheKey(tt.authContext)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetOrganizationCached_DoubleCheckHit(t *testing.T) {
	ClearOrganizationCache()

	ctrl := gomock.NewController(t)
	mock := NewMockGetter(ctrl)

	// Expect zero calls because we pre-populate the cache.
	mock.EXPECT().
		GetOrganization(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	restore := SetGetter(mock)
	defer restore()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	// Pre-populate the cache directly to simulate another goroutine having cached the result
	// between our read-lock miss and write-lock acquisition (the double-check branch).
	cacheKey := getCacheKey(nil)
	organizationCacheMu.Lock()
	organizationCache[cacheKey] = &cachedOrganization{
		info: &OrganizationInfo{ID: "o-pre-populated"},
		err:  nil,
	}
	organizationCacheMu.Unlock()

	// This call should hit the double-check branch and return the pre-populated value.
	info, err := GetOrganizationCached(ctx, atmosConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, "o-pre-populated", info.ID)
}

func TestSetGetter_NilFallback(t *testing.T) {
	// Save original getter to restore at end.
	originalGetter := getter
	defer func() { getter = originalGetter }()

	// Calling SetGetter(nil) should reset getter to defaultGetter, not set it to nil.
	SetGetter(nil)
	assert.IsType(t, &defaultGetter{}, getter)
	assert.NotNil(t, getter)
}

func TestDefaultGetter_GetOrganization_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockorganizationsAPI(ctrl)

	mockClient.EXPECT().
		DescribeOrganization(gomock.Any(), gomock.Any()).
		Return(&organizations.DescribeOrganizationOutput{
			Organization: &types.Organization{
				Id:                 aws.String("o-test123"),
				Arn:                aws.String("arn:aws:organizations::111111111111:organization/o-test123"),
				MasterAccountId:    aws.String("111111111111"),
				MasterAccountEmail: aws.String("master@example.com"),
			},
		}, nil).
		Times(1)

	restore := withMockOrgClient(t, mockClient, nil)
	defer restore()

	d := &defaultGetter{}
	info, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, nil)

	require.NoError(t, err)
	assert.Equal(t, "o-test123", info.ID)
	assert.Equal(t, "arn:aws:organizations::111111111111:organization/o-test123", info.Arn)
	assert.Equal(t, "111111111111", info.MasterAccountID)
	assert.Equal(t, "master@example.com", info.MasterAccountEmail)
}

func TestDefaultGetter_GetOrganization_NilFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockorganizationsAPI(ctrl)

	mockClient.EXPECT().
		DescribeOrganization(gomock.Any(), gomock.Any()).
		Return(&organizations.DescribeOrganizationOutput{
			Organization: &types.Organization{
				Id: aws.String("o-partial"),
				// Arn, MasterAccountId, MasterAccountEmail are nil.
			},
		}, nil).
		Times(1)

	restore := withMockOrgClient(t, mockClient, nil)
	defer restore()

	d := &defaultGetter{}
	info, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, nil)

	require.NoError(t, err)
	assert.Equal(t, "o-partial", info.ID)
	assert.Empty(t, info.Arn)
	assert.Empty(t, info.MasterAccountID)
	assert.Empty(t, info.MasterAccountEmail)
}

func TestDefaultGetter_GetOrganization_ConfigError(t *testing.T) {
	configErr := errors.New("config load failed")
	restore := withMockOrgClient(t, nil, configErr)
	defer restore()

	d := &defaultGetter{}
	_, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsDescribeOrganization)
	assert.Contains(t, err.Error(), "config load failed")
}

func TestDefaultGetter_GetOrganization_DescribeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockorganizationsAPI(ctrl)

	describeErr := errors.New("access denied")
	mockClient.EXPECT().
		DescribeOrganization(gomock.Any(), gomock.Any()).
		Return(nil, describeErr).
		Times(1)

	restore := withMockOrgClient(t, mockClient, nil)
	defer restore()

	d := &defaultGetter{}
	_, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsDescribeOrganization)
	assert.ErrorIs(t, err, describeErr)
}

func TestDefaultGetter_GetOrganization_NotInOrg(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockorganizationsAPI(ctrl)

	notInOrgErr := &types.AWSOrganizationsNotInUseException{
		Message: aws.String("account not in org"),
	}
	mockClient.EXPECT().
		DescribeOrganization(gomock.Any(), gomock.Any()).
		Return(nil, notInOrgErr).
		Times(1)

	restore := withMockOrgClient(t, mockClient, nil)
	defer restore()

	d := &defaultGetter{}
	_, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsDescribeOrganization)
	assert.Contains(t, err.Error(), "not a member of an organization")
}

func TestDefaultGetter_GetOrganization_NilOrgPayload(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockorganizationsAPI(ctrl)

	mockClient.EXPECT().
		DescribeOrganization(gomock.Any(), gomock.Any()).
		Return(&organizations.DescribeOrganizationOutput{
			Organization: nil,
		}, nil).
		Times(1)

	restore := withMockOrgClient(t, mockClient, nil)
	defer restore()

	d := &defaultGetter{}
	_, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsDescribeOrganization)
	assert.Contains(t, err.Error(), "empty organization payload")
}

func TestDefaultGetter_GetOrganization_PassesAuthContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockorganizationsAPI(ctrl)

	mockClient.EXPECT().
		DescribeOrganization(gomock.Any(), gomock.Any()).
		Return(&organizations.DescribeOrganizationOutput{
			Organization: &types.Organization{
				Id: aws.String("o-auth"),
			},
		}, nil).
		Times(1)

	var capturedAuth *schema.AWSAuthContext
	oldLoadConfig := loadAWSConfig
	oldNewClient := newOrgClient

	loadAWSConfig = func(
		_ context.Context, _, _ string, _ time.Duration, authContext *schema.AWSAuthContext,
	) (aws.Config, error) {
		capturedAuth = authContext
		return aws.Config{}, nil
	}
	newOrgClient = func(_ aws.Config) organizationsAPI { return mockClient }

	defer func() {
		loadAWSConfig = oldLoadConfig
		newOrgClient = oldNewClient
	}()

	authContext := &schema.AWSAuthContext{
		Profile:         "test-profile",
		CredentialsFile: "/test/credentials",
		ConfigFile:      "/test/config",
	}

	d := &defaultGetter{}
	info, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, authContext)

	require.NoError(t, err)
	assert.Equal(t, "o-auth", info.ID)
	assert.Equal(t, authContext, capturedAuth)
}

func TestClearOrganizationCache(t *testing.T) {
	ClearOrganizationCache()

	ctrl := gomock.NewController(t)
	mock := NewMockGetter(ctrl)

	mock.EXPECT().
		GetOrganization(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&OrganizationInfo{ID: "o-clear-test"}, nil).
		Times(1)

	restore := SetGetter(mock)
	defer restore()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	// Populate cache.
	_, err := GetOrganizationCached(ctx, atmosConfig, nil)
	require.NoError(t, err)

	// Clear cache.
	ClearOrganizationCache()

	// Verify cache is empty by checking the internal map.
	organizationCacheMu.RLock()
	assert.Empty(t, organizationCache)
	organizationCacheMu.RUnlock()
}
