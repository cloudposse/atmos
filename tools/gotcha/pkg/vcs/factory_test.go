package vcs_test

import (
	"context"
	"os"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/vcs"
	_ "github.com/cloudposse/atmos/tools/gotcha/pkg/vcs/github" // Register GitHub provider
	"github.com/cloudposse/atmos/tools/gotcha/pkg/vcs/mock"     // Import mock provider
	_ "github.com/cloudposse/atmos/tools/gotcha/pkg/vcs/mock"   // Register mock provider
	"github.com/stretchr/testify/assert"
)

func TestMockProviderRegistration(t *testing.T) {
	// Verify that the mock provider is registered
	platforms := vcs.GetSupportedPlatforms()
	hasMock := false
	for _, p := range platforms {
		if p == vcs.Platform("mock") {
			hasMock = true
			break
		}
	}
	assert.True(t, hasMock, "Mock provider should be registered")
}

func TestDetectProvider(t *testing.T) {
	logger := log.New(nil)

	t.Run("no provider detected", func(t *testing.T) {
		// Clear any environment variables that might trigger detection
		oldGithubActions := os.Getenv("GITHUB_ACTIONS")
		oldUseMock := os.Getenv("GOTCHA_USE_MOCK")
		oldVcsPlatform := os.Getenv("GOTCHA_VCS_PLATFORM")

		os.Unsetenv("GITHUB_ACTIONS")
		os.Unsetenv("GOTCHA_USE_MOCK")
		os.Unsetenv("GOTCHA_VCS_PLATFORM")

		defer func() {
			if oldGithubActions != "" {
				os.Setenv("GITHUB_ACTIONS", oldGithubActions)
			}
			if oldUseMock != "" {
				os.Setenv("GOTCHA_USE_MOCK", oldUseMock)
			}
			if oldVcsPlatform != "" {
				os.Setenv("GOTCHA_VCS_PLATFORM", oldVcsPlatform)
			}
		}()

		provider := vcs.DetectProvider(logger)
		assert.Nil(t, provider)
	})

	t.Run("mock provider via environment", func(t *testing.T) {
		oldUseMock := os.Getenv("GOTCHA_USE_MOCK")
		os.Setenv("GOTCHA_USE_MOCK", "true")
		defer func() {
			if oldUseMock != "" {
				os.Setenv("GOTCHA_USE_MOCK", oldUseMock)
			} else {
				os.Unsetenv("GOTCHA_USE_MOCK")
			}
		}()

		provider := vcs.DetectProvider(logger)
		assert.NotNil(t, provider, "Provider should not be nil when GOTCHA_USE_MOCK=true")
		if provider != nil {
			assert.Equal(t, vcs.Platform("mock"), provider.GetPlatform())
		}
	})

	t.Run("manual platform override", func(t *testing.T) {
		oldVcsPlatform := os.Getenv("GOTCHA_VCS_PLATFORM")
		oldUseMock := os.Getenv("GOTCHA_USE_MOCK")

		os.Setenv("GOTCHA_VCS_PLATFORM", "mock")
		os.Setenv("GOTCHA_USE_MOCK", "true")

		defer func() {
			if oldVcsPlatform != "" {
				os.Setenv("GOTCHA_VCS_PLATFORM", oldVcsPlatform)
			} else {
				os.Unsetenv("GOTCHA_VCS_PLATFORM")
			}
			if oldUseMock != "" {
				os.Setenv("GOTCHA_USE_MOCK", oldUseMock)
			} else {
				os.Unsetenv("GOTCHA_USE_MOCK")
			}
		}()

		provider := vcs.DetectProvider(logger)
		assert.NotNil(t, provider)
		assert.Equal(t, vcs.Platform("mock"), provider.GetPlatform())
	})
}

func TestGetProvider(t *testing.T) {
	logger := log.New(nil)

	t.Run("get mock provider directly", func(t *testing.T) {
		provider := vcs.GetProvider(vcs.Platform("mock"), logger)
		assert.NotNil(t, provider, "Should be able to get mock provider directly")
		if provider != nil {
			assert.Equal(t, vcs.Platform("mock"), provider.GetPlatform())
			// Verify it's available when GOTCHA_USE_MOCK is set
			oldUseMock := os.Getenv("GOTCHA_USE_MOCK")
			os.Setenv("GOTCHA_USE_MOCK", "true")
			assert.True(t, provider.IsAvailable(), "Mock provider should be available when GOTCHA_USE_MOCK=true")
			if oldUseMock != "" {
				os.Setenv("GOTCHA_USE_MOCK", oldUseMock)
			} else {
				os.Unsetenv("GOTCHA_USE_MOCK")
			}
		}
	})

	t.Run("get unknown provider", func(t *testing.T) {
		provider := vcs.GetProvider(vcs.Platform("unknown"), logger)
		assert.Nil(t, provider)
	})
}

func TestRegisterProvider(t *testing.T) {
	logger := log.New(nil)

	// Create a custom test provider
	testPlatform := vcs.Platform("test-custom")

	// Register it
	vcs.RegisterProvider(testPlatform, func(l *log.Logger) vcs.Provider {
		return mock.NewMockProvider(l)
	})

	// Try to get it
	provider := vcs.GetProvider(testPlatform, logger)
	assert.NotNil(t, provider)
}

func TestGetSupportedPlatforms(t *testing.T) {
	platforms := vcs.GetSupportedPlatforms()
	assert.NotEmpty(t, platforms)

	// At minimum, we should have mock and github registered
	hasMock := false
	hasGitHub := false

	for _, p := range platforms {
		if p == vcs.Platform("mock") {
			hasMock = true
		}
		if p == vcs.PlatformGitHub {
			hasGitHub = true
		}
	}

	assert.True(t, hasMock, "mock provider should be registered")
	assert.True(t, hasGitHub, "github provider should be registered")
}

func TestIsCI(t *testing.T) {
	t.Run("not in CI", func(t *testing.T) {
		// Clear all CI environment variables
		ciVars := []string{
			"CI", "CONTINUOUS_INTEGRATION", "GITHUB_ACTIONS",
			"GITLAB_CI", "BITBUCKET_PIPELINES", "JENKINS_URL",
			"CIRCLECI", "TRAVIS", "SYSTEM_TEAMFOUNDATIONCOLLECTIONURI",
		}

		oldValues := make(map[string]string)
		for _, v := range ciVars {
			oldValues[v] = os.Getenv(v)
			os.Unsetenv(v)
		}

		defer func() {
			for k, v := range oldValues {
				if v != "" {
					os.Setenv(k, v)
				}
			}
		}()

		assert.False(t, vcs.IsCI())
	})

	t.Run("in CI", func(t *testing.T) {
		oldCI := os.Getenv("CI")
		os.Setenv("CI", "true")
		defer func() {
			if oldCI != "" {
				os.Setenv("CI", oldCI)
			} else {
				os.Unsetenv("CI")
			}
		}()

		assert.True(t, vcs.IsCI())
	})
}

func TestMockProviderIntegration(t *testing.T) {
	logger := log.New(nil)

	// Set up mock provider to be detectable
	oldUseMock := os.Getenv("GOTCHA_USE_MOCK")
	os.Setenv("GOTCHA_USE_MOCK", "true")
	defer func() {
		if oldUseMock != "" {
			os.Setenv("GOTCHA_USE_MOCK", oldUseMock)
		} else {
			os.Unsetenv("GOTCHA_USE_MOCK")
		}
	}()

	// Detect provider
	provider := vcs.DetectProvider(logger)
	assert.NotNil(t, provider)
	assert.Equal(t, vcs.Platform("mock"), provider.GetPlatform())

	// Get context
	ctx, err := provider.DetectContext()
	assert.NoError(t, err)
	assert.NotNil(t, ctx)

	// Create comment manager
	cm := provider.CreateCommentManager(ctx, logger)
	assert.NotNil(t, cm)

	// Test posting a comment
	err = cm.PostOrUpdateComment(context.Background(), ctx, "Test comment from integration test")
	assert.NoError(t, err)

	// Get job summary writer
	jsw := provider.GetJobSummaryWriter()
	assert.NotNil(t, jsw)

	// Write a summary
	path, err := jsw.WriteJobSummary("# Integration Test Summary")
	assert.NoError(t, err)
	assert.NotEmpty(t, path)

	// Verify we can cast to mock provider and check internal state
	if mockProvider, ok := provider.(*mock.MockProvider); ok {
		comments := mockProvider.GetComments()
		assert.NotEmpty(t, comments)

		summaries := mockProvider.GetWrittenSummaries()
		assert.NotEmpty(t, summaries)
	}
}
