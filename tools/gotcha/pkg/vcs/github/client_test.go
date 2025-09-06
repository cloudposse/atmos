package github

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name         string
		token        string
		envToken     string
		expectAuth   bool
		expectedType string
	}{
		{
			name:         "with explicit token",
			token:        "explicit-token",
			expectAuth:   true,
			expectedType: "*github.RealClient",
		},
		{
			name:         "with env token",
			token:        "",
			envToken:     "env-token",
			expectAuth:   true,
			expectedType: "*github.RealClient",
		},
		{
			name:         "no token",
			token:        "",
			envToken:     "",
			expectAuth:   false,
			expectedType: "*github.RealClient",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			oldToken := os.Getenv("GITHUB_TOKEN")
			defer func() {
				if oldToken != "" {
					os.Setenv("GITHUB_TOKEN", oldToken)
				} else {
					os.Unsetenv("GITHUB_TOKEN")
				}
			}()

			if tt.envToken != "" {
				os.Setenv("GITHUB_TOKEN", tt.envToken)
			} else {
				os.Unsetenv("GITHUB_TOKEN")
			}

			client := NewClient(tt.token)

			assert.NotNil(t, client)
			assert.IsType(t, &RealClient{}, client)

			realClient, ok := client.(*RealClient)
			require.True(t, ok)
			assert.NotNil(t, realClient.client)
		})
	}
}

func TestRealClientMethods(t *testing.T) {
	// Skip if no GitHub token is available
	if os.Getenv("GITHUB_TOKEN") == "" && os.Getenv("GOTCHA_GITHUB_TOKEN") == "" {
		t.Skipf("Skipping test: GITHUB_TOKEN not set (required for GitHub API calls)")
	}
	
	// Create a real client for interface verification
	client := NewClient("")
	realClient, ok := client.(*RealClient)
	require.True(t, ok)
	require.NotNil(t, realClient.client)

	ctx := context.Background()

	// Test that methods exist and can be called (they'll fail due to no auth/invalid repo, but that's expected)
	t.Run("ListIssueComments method exists", func(t *testing.T) {
		// This will fail due to invalid repo, but verifies the method signature
		_, _, err := client.ListIssueComments(ctx, "invalid", "repo", 1, nil)
		// We expect an error here since it's not a real repo, but the method should exist
		assert.Error(t, err)
	})

	t.Run("CreateComment method exists", func(t *testing.T) {
		// This will fail due to invalid repo, but verifies the method signature
		_, _, err := client.CreateComment(ctx, "invalid", "repo", 1, nil)
		// We expect an error here since it's not a real repo, but the method should exist
		assert.Error(t, err)
	})

	t.Run("UpdateComment method exists", func(t *testing.T) {
		// This will fail due to invalid repo, but verifies the method signature
		_, _, err := client.UpdateComment(ctx, "invalid", "repo", 1, nil)
		// We expect an error here since it's not a real repo, but the method should exist
		assert.Error(t, err)
	})
}

func TestClientInterface(t *testing.T) {
	// Verify that RealClient implements the Client interface
	var _ Client = &RealClient{}
	var _ Client = &MockClient{}

	// Test that both implementations can be used interchangeably
	clients := []Client{
		NewClient("test-token"),
		NewMockClient(),
	}

	for i, client := range clients {
		t.Run(fmt.Sprintf("client_%d", i), func(t *testing.T) {
			// Skip real client tests if no GitHub token
			if _, isReal := client.(*RealClient); isReal {
				if os.Getenv("GITHUB_TOKEN") == "" && os.Getenv("GOTCHA_GITHUB_TOKEN") == "" {
					t.Skipf("Skipping real client test: GITHUB_TOKEN not set")
				}
			}
			
			ctx := context.Background()

			// Verify interface methods are available
			assert.NotPanics(t, func() {
				_, _, _ = client.ListIssueComments(ctx, "owner", "repo", 1, nil)
				_, _, _ = client.CreateComment(ctx, "owner", "repo", 1, nil)
				_, _, _ = client.UpdateComment(ctx, "owner", "repo", 1, nil)
			})
		})
	}
}
