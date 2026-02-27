package organization

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

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
