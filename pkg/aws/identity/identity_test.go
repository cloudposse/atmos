package identity

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// mockGetter is a test implementation of Getter.
type mockGetter struct {
	identity *CallerIdentity
	err      error
	calls    int
}

func (m *mockGetter) GetCallerIdentity(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	authContext *schema.AWSAuthContext,
) (*CallerIdentity, error) {
	m.calls++
	return m.identity, m.err
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
			name:        "empty auth context",
			authContext: &schema.AWSAuthContext{},
			expected:    "::",
		},
		{
			name: "full auth context",
			authContext: &schema.AWSAuthContext{
				Profile:         "prod",
				CredentialsFile: "/home/user/.aws/credentials",
				ConfigFile:      "/home/user/.aws/config",
			},
			expected: "prod:/home/user/.aws/credentials:/home/user/.aws/config",
		},
		{
			name: "partial auth context",
			authContext: &schema.AWSAuthContext{
				Profile: "dev",
			},
			expected: "dev::",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCacheKey(tt.authContext)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetCallerIdentityCached(t *testing.T) {
	// Clear cache before test.
	ClearIdentityCache()

	// Set up mock getter.
	mock := &mockGetter{
		identity: &CallerIdentity{
			Account: "123456789012",
			Arn:     "arn:aws:iam::123456789012:user/test",
			UserID:  "AIDAEXAMPLE",
			Region:  "us-west-2",
		},
	}

	// Replace the global getter with our mock.
	restore := SetGetter(mock)
	defer restore()

	ctx := context.Background()

	// First call should hit the mock.
	identity, err := GetCallerIdentityCached(ctx, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "123456789012", identity.Account)
	assert.Equal(t, 1, mock.calls)

	// Second call should use cache.
	identity2, err := GetCallerIdentityCached(ctx, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, identity, identity2)
	assert.Equal(t, 1, mock.calls, "should not call mock again due to cache")

	// Call with different auth context should hit the mock again.
	authContext := &schema.AWSAuthContext{Profile: "other"}
	identity3, err := GetCallerIdentityCached(ctx, nil, authContext)
	require.NoError(t, err)
	assert.Equal(t, "123456789012", identity3.Account)
	assert.Equal(t, 2, mock.calls, "should call mock for different auth context")
}

func TestClearIdentityCache(t *testing.T) {
	// Clear cache before test to ensure isolation.
	ClearIdentityCache()

	// Set up mock getter.
	mock := &mockGetter{
		identity: &CallerIdentity{
			Account: "123456789012",
		},
	}

	restore := SetGetter(mock)
	defer restore()

	ctx := context.Background()

	// First call.
	_, err := GetCallerIdentityCached(ctx, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, mock.calls)

	// Second call should use cache.
	_, err = GetCallerIdentityCached(ctx, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, mock.calls)

	// Clear cache.
	ClearIdentityCache()

	// Third call should hit the mock again.
	_, err = GetCallerIdentityCached(ctx, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, mock.calls, "should call mock after cache clear")
}

func TestSetGetter(t *testing.T) {
	originalGetter := getter

	mock := &mockGetter{}
	restore := SetGetter(mock)

	assert.Same(t, mock, getter)

	restore()

	assert.Same(t, originalGetter, getter)
}

func TestCallerIdentity(t *testing.T) {
	identity := &CallerIdentity{
		Account: "123456789012",
		Arn:     "arn:aws:iam::123456789012:user/test",
		UserID:  "AIDAEXAMPLE",
		Region:  "us-east-1",
	}

	assert.Equal(t, "123456789012", identity.Account)
	assert.Equal(t, "arn:aws:iam::123456789012:user/test", identity.Arn)
	assert.Equal(t, "AIDAEXAMPLE", identity.UserID)
	assert.Equal(t, "us-east-1", identity.Region)
}

func TestGetCallerIdentityCached_Error(t *testing.T) {
	// Clear cache before test.
	ClearIdentityCache()

	// Set up mock getter that returns an error.
	expectedErr := assert.AnError
	mock := &mockGetter{
		identity: nil,
		err:      expectedErr,
	}

	restore := SetGetter(mock)
	defer restore()

	ctx := context.Background()

	// First call should hit the mock and return error.
	_, err := GetCallerIdentityCached(ctx, nil, nil)
	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Equal(t, 1, mock.calls)

	// Second call should return cached error without calling mock again.
	_, err = GetCallerIdentityCached(ctx, nil, nil)
	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Equal(t, 1, mock.calls, "should use cached error")
}

func TestGetCallerIdentityCached_ConcurrentAccess(t *testing.T) {
	// Clear cache before test.
	ClearIdentityCache()

	// Set up mock getter.
	mock := &mockGetter{
		identity: &CallerIdentity{
			Account: "123456789012",
			Arn:     "arn:aws:iam::123456789012:user/test",
			UserID:  "AIDAEXAMPLE",
			Region:  "us-west-2",
		},
	}

	restore := SetGetter(mock)
	defer restore()

	ctx := context.Background()
	numGoroutines := 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	results := make([]*CallerIdentity, numGoroutines)
	errors := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx], errors[idx] = GetCallerIdentityCached(ctx, nil, nil)
		}(i)
	}

	wg.Wait()

	// All results should be successful.
	for i := 0; i < numGoroutines; i++ {
		require.NoError(t, errors[i])
		assert.Equal(t, "123456789012", results[i].Account)
	}

	// Mock should have been called only once due to caching.
	assert.Equal(t, 1, mock.calls)
}

func TestGetCacheKey_AuthContextWithRegion(t *testing.T) {
	authContext := &schema.AWSAuthContext{
		Profile:         "dev",
		CredentialsFile: "/path/to/creds",
		ConfigFile:      "/path/to/config",
		Region:          "eu-west-1", // Region is not included in cache key.
	}

	// Region should not affect the cache key since it's handled separately.
	result := getCacheKey(authContext)
	assert.Equal(t, "dev:/path/to/creds:/path/to/config", result)
}

func TestDefaultGetter_GetCallerIdentity(t *testing.T) {
	// This test would require actual AWS credentials, so we skip in unit tests.
	// It's here for documentation and can be run manually with credentials.
	t.Skip("Requires actual AWS credentials - run manually for integration testing")
}
