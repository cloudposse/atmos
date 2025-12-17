package identity

import (
	"context"
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
