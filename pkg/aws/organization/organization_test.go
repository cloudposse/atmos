package organization

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// mockGetter is a test implementation of Getter.
type mockOrgGetter struct {
	info *OrganizationInfo
	err  error
}

func (m *mockOrgGetter) GetOrganization(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	authContext *schema.AWSAuthContext,
) (*OrganizationInfo, error) {
	return m.info, m.err
}

// countingOrgGetter wraps another getter and counts calls.
type countingOrgGetter struct {
	wrapped   Getter
	callCount *int
}

func (c *countingOrgGetter) GetOrganization(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	authContext *schema.AWSAuthContext,
) (*OrganizationInfo, error) {
	*c.callCount++
	return c.wrapped.GetOrganization(ctx, atmosConfig, authContext)
}

func TestGetOrganizationCached_Success(t *testing.T) {
	ClearOrganizationCache()

	mock := &mockOrgGetter{
		info: &OrganizationInfo{
			ID:                 "o-abc123",
			Arn:                "arn:aws:organizations::111111111111:organization/o-abc123",
			MasterAccountID:    "111111111111",
			MasterAccountEmail: "master@example.com",
		},
	}

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

	callCount := 0
	mock := &mockOrgGetter{
		info: &OrganizationInfo{
			ID:              "o-cached",
			Arn:             "arn:aws:organizations::222222222222:organization/o-cached",
			MasterAccountID: "222222222222",
		},
	}

	counting := &countingOrgGetter{
		wrapped:   mock,
		callCount: &callCount,
	}

	restore := SetGetter(counting)
	defer restore()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	// First call should hit the mock.
	info1, err := GetOrganizationCached(ctx, atmosConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, "o-cached", info1.ID)
	assert.Equal(t, 1, callCount)

	// Second call with same auth context should use cache.
	info2, err := GetOrganizationCached(ctx, atmosConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, "o-cached", info2.ID)
	assert.Equal(t, 1, callCount, "Second call should use cache")

	// Call with different auth context should call getter again.
	differentAuth := &schema.AWSAuthContext{
		Profile:         "different-profile",
		CredentialsFile: "/different/path",
	}
	info3, err := GetOrganizationCached(ctx, atmosConfig, differentAuth)
	require.NoError(t, err)
	assert.Equal(t, "o-cached", info3.ID)
	assert.Equal(t, 2, callCount, "Different auth context should invoke getter")

	// Clear cache and verify next call hits mock.
	ClearOrganizationCache()
	info4, err := GetOrganizationCached(ctx, atmosConfig, nil)
	require.NoError(t, err)
	assert.Equal(t, "o-cached", info4.ID)
	assert.Equal(t, 3, callCount, "After cache clear, should invoke getter")
}

func TestGetOrganizationCached_ErrorCaching(t *testing.T) {
	ClearOrganizationCache()

	callCount := 0
	expectedErr := errors.New("mock organization error")
	mock := &mockOrgGetter{
		info: nil,
		err:  expectedErr,
	}

	counting := &countingOrgGetter{
		wrapped:   mock,
		callCount: &callCount,
	}

	restore := SetGetter(counting)
	defer restore()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	// First call should return error and cache it.
	_, err := GetOrganizationCached(ctx, atmosConfig, nil)
	require.Error(t, err)
	assert.Equal(t, 1, callCount)

	// Second call should return cached error.
	_, err = GetOrganizationCached(ctx, atmosConfig, nil)
	require.Error(t, err)
	assert.Equal(t, 1, callCount, "Errors should be cached too")
}

func TestGetOrganizationCached_Concurrent(t *testing.T) {
	ClearOrganizationCache()

	callCount := 0
	mock := &mockOrgGetter{
		info: &OrganizationInfo{
			ID:              "o-concurrent",
			Arn:             "arn:aws:organizations::333333333333:organization/o-concurrent",
			MasterAccountID: "333333333333",
		},
	}

	counting := &countingOrgGetter{
		wrapped:   mock,
		callCount: &callCount,
	}

	restore := SetGetter(counting)
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

	// Despite concurrent access, should only call getter once due to caching.
	assert.Equal(t, 1, callCount, "Concurrent access should result in only one getter call")
}

func TestSetGetter_Restore(t *testing.T) {
	ClearOrganizationCache()

	// Save original getter type.
	originalGetter := getter

	mock := &mockOrgGetter{
		info: &OrganizationInfo{ID: "o-mock"},
	}

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

	mock := &mockOrgGetter{
		info: &OrganizationInfo{ID: "o-doublecheck"},
	}

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

	mock := &mockOrgGetter{
		info: &OrganizationInfo{ID: "o-clear-test"},
	}

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
