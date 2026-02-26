package okta

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOktaFileManager_DefaultPath(t *testing.T) {
	// Use temp directory to avoid affecting user's home directory.
	tempDir := t.TempDir()

	mgr, err := NewOktaFileManager(tempDir, "")
	require.NoError(t, err)
	assert.Equal(t, tempDir, mgr.GetBaseDir())
}

func TestNewOktaFileManager_WithRealm(t *testing.T) {
	tempDir := t.TempDir()

	mgr, err := NewOktaFileManager(tempDir, "test-realm")
	require.NoError(t, err)
	// When custom base path is provided, realm is not appended.
	assert.Equal(t, tempDir, mgr.GetBaseDir())
}

func TestOktaFileManager_GetProviderDir(t *testing.T) {
	mgr := &OktaFileManager{baseDir: "/home/user/.config/atmos/okta"}
	assert.Equal(t, filepath.Join("/home/user/.config/atmos/okta", "okta-oidc"), mgr.GetProviderDir("okta-oidc"))
}

func TestOktaFileManager_GetTokensPath(t *testing.T) {
	mgr := &OktaFileManager{baseDir: "/home/user/.config/atmos/okta"}
	expected := filepath.Join("/home/user/.config/atmos/okta", "okta-oidc", "tokens.json")
	assert.Equal(t, expected, mgr.GetTokensPath("okta-oidc"))
}

func TestOktaFileManager_WriteAndLoadTokens(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	tokens := &OktaTokens{
		AccessToken:  "test-access-token",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshToken: "test-refresh-token",
		IDToken:      "test-id-token",
		Scope:        "openid profile",
	}

	// Write tokens.
	err = mgr.WriteTokens("test-provider", tokens)
	require.NoError(t, err)

	// Verify file exists.
	assert.True(t, mgr.TokensExist("test-provider"))

	// Load tokens.
	loadedTokens, err := mgr.LoadTokens("test-provider")
	require.NoError(t, err)
	assert.Equal(t, tokens.AccessToken, loadedTokens.AccessToken)
	assert.Equal(t, tokens.TokenType, loadedTokens.TokenType)
	assert.Equal(t, tokens.RefreshToken, loadedTokens.RefreshToken)
	assert.Equal(t, tokens.IDToken, loadedTokens.IDToken)
	assert.Equal(t, tokens.Scope, loadedTokens.Scope)
}

func TestOktaFileManager_Cleanup(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	tokens := &OktaTokens{
		AccessToken: "test-access-token",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(time.Hour),
	}

	// Write tokens.
	err = mgr.WriteTokens("test-provider", tokens)
	require.NoError(t, err)
	assert.True(t, mgr.TokensExist("test-provider"))

	// Cleanup.
	err = mgr.Cleanup("test-provider")
	require.NoError(t, err)
	assert.False(t, mgr.TokensExist("test-provider"))
}

func TestOktaFileManager_CleanupNonExistent(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	// Cleanup non-existent provider should not error.
	err = mgr.Cleanup("non-existent-provider")
	require.NoError(t, err)
}

func TestOktaFileManager_TokensExist_False(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	assert.False(t, mgr.TokensExist("non-existent-provider"))
}

func TestOktaFileManager_LoadTokens_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	_, err = mgr.LoadTokens("non-existent-provider")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTokensFileNotFound)
}

func TestOktaTokens_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		expected  bool
	}{
		{
			name:      "not expired",
			expiresAt: time.Now().Add(time.Hour),
			expected:  false,
		},
		{
			name:      "expired",
			expiresAt: time.Now().Add(-time.Hour),
			expected:  true,
		},
		{
			name:      "expires within 5 minutes (considered expired)",
			expiresAt: time.Now().Add(3 * time.Minute),
			expected:  true,
		},
		{
			name:      "zero time (expired)",
			expiresAt: time.Time{},
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := &OktaTokens{ExpiresAt: tt.expiresAt}
			assert.Equal(t, tt.expected, tokens.IsExpired())
		})
	}
}

func TestNewOktaFileManager_EmptyBasePath(t *testing.T) {
	// With empty basePath and no realm, should use default home dir.
	mgr, err := NewOktaFileManager("", "")
	require.NoError(t, err)
	assert.Contains(t, mgr.GetBaseDir(), "okta")
}

func TestNewOktaFileManager_EmptyBasePathWithRealm(t *testing.T) {
	// With empty basePath and realm, should include realm in path.
	mgr, err := NewOktaFileManager("", "my-realm")
	require.NoError(t, err)
	assert.Contains(t, mgr.GetBaseDir(), "my-realm")
	assert.Contains(t, mgr.GetBaseDir(), "okta")
}

func TestOktaFileManager_GetDisplayPath(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	// Display path for a non-home dir should return the dir itself.
	displayPath := mgr.GetDisplayPath()
	assert.NotEmpty(t, displayPath)
}

func TestOktaFileManager_WriteTokens_NilTokens(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	err = mgr.WriteTokens("test-provider", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWriteTokensFile)
}

func TestOktaFileManager_CleanupAll(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	// Write tokens for two providers.
	tokens := &OktaTokens{
		AccessToken: "token",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	err = mgr.WriteTokens("provider-a", tokens)
	require.NoError(t, err)
	err = mgr.WriteTokens("provider-b", tokens)
	require.NoError(t, err)
	assert.True(t, mgr.TokensExist("provider-a"))
	assert.True(t, mgr.TokensExist("provider-b"))

	// Cleanup all.
	err = mgr.CleanupAll()
	require.NoError(t, err)
	assert.False(t, mgr.TokensExist("provider-a"))
	assert.False(t, mgr.TokensExist("provider-b"))
}

func TestOktaFileManager_CleanupAll_NonExistent(t *testing.T) {
	// CleanupAll on a non-existent directory should be a no-op.
	mgr := &OktaFileManager{baseDir: filepath.Join(t.TempDir(), "does-not-exist")}
	err := mgr.CleanupAll()
	require.NoError(t, err)
}

func TestOktaFileManager_DeleteIdentity(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	// Write tokens.
	tokens := &OktaTokens{
		AccessToken: "token",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	err = mgr.WriteTokens("test-provider", tokens)
	require.NoError(t, err)
	assert.True(t, mgr.TokensExist("test-provider"))

	// Delete identity.
	err = mgr.DeleteIdentity(context.Background(), "test-provider", "test-identity")
	require.NoError(t, err)
	assert.False(t, mgr.TokensExist("test-provider"))
}

func TestOktaFileManager_DeleteIdentity_NonExistent(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	// Delete non-existent identity should be a no-op.
	err = mgr.DeleteIdentity(context.Background(), "non-existent", "test-identity")
	require.NoError(t, err)
}

func TestTokenResponse_ToOktaTokens(t *testing.T) {
	resp := &TokenResponse{
		AccessToken:  "access-token",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		RefreshToken: "refresh-token",
		IDToken:      "id-token",
		Scope:        "openid profile",
	}

	tokens := resp.ToOktaTokens()
	assert.Equal(t, "access-token", tokens.AccessToken)
	assert.Equal(t, "Bearer", tokens.TokenType)
	assert.Equal(t, 3600, tokens.ExpiresIn)
	assert.Equal(t, "refresh-token", tokens.RefreshToken)
	assert.Equal(t, "id-token", tokens.IDToken)
	assert.Equal(t, "openid profile", tokens.Scope)
	// ExpiresAt should be approximately 1 hour from now.
	assert.WithinDuration(t, time.Now().Add(time.Hour), tokens.ExpiresAt, 5*time.Second)
	// RefreshTokenExpiresAt should be approximately 7 days from now.
	assert.WithinDuration(t, time.Now().Add(7*24*time.Hour), tokens.RefreshTokenExpiresAt, 5*time.Second)
}

func TestTokenResponse_ToOktaTokens_NoRefresh(t *testing.T) {
	resp := &TokenResponse{
		AccessToken: "access-token",
		TokenType:   "Bearer",
		ExpiresIn:   3600,
	}

	tokens := resp.ToOktaTokens()
	assert.Empty(t, tokens.RefreshToken)
	assert.True(t, tokens.RefreshTokenExpiresAt.IsZero())
}

func TestOktaTokens_CanRefresh(t *testing.T) {
	tests := []struct {
		name                  string
		refreshToken          string
		refreshTokenExpiresAt time.Time
		expected              bool
	}{
		{
			name:         "no refresh token",
			refreshToken: "",
			expected:     false,
		},
		{
			name:                  "refresh token with no expiration",
			refreshToken:          "test-refresh",
			refreshTokenExpiresAt: time.Time{},
			expected:              true,
		},
		{
			name:                  "refresh token not expired",
			refreshToken:          "test-refresh",
			refreshTokenExpiresAt: time.Now().Add(time.Hour),
			expected:              true,
		},
		{
			name:                  "refresh token expired",
			refreshToken:          "test-refresh",
			refreshTokenExpiresAt: time.Now().Add(-time.Hour),
			expected:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := &OktaTokens{
				RefreshToken:          tt.refreshToken,
				RefreshTokenExpiresAt: tt.refreshTokenExpiresAt,
			}
			assert.Equal(t, tt.expected, tokens.CanRefresh())
		})
	}
}
