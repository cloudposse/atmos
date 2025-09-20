package ci_test

import (
	"context"
	"os"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/ci"
	_ "github.com/cloudposse/atmos/tools/gotcha/pkg/ci/github" // Register GitHub integration
	"github.com/cloudposse/atmos/tools/gotcha/pkg/ci/mock"     // Import and register mock integration
	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestMockIntegrationRegistration(t *testing.T) {
	// Verify that the mock integration is registered
	providers := ci.GetSupportedProviders()
	hasMock := false
	for _, p := range providers {
		if p == "mock" {
			hasMock = true
			break
		}
	}
	assert.True(t, hasMock, "Mock integration should be registered")
}

func TestDetectIntegration(t *testing.T) {
	logger := log.New(nil)

	t.Run("no integration detected", func(t *testing.T) {
		// Clear any environment variables that might trigger detection
		oldGithubActions := os.Getenv("GITHUB_ACTIONS")
		oldUseMock := os.Getenv("GOTCHA_USE_MOCK")
		oldCiProvider := os.Getenv("GOTCHA_CI_PROVIDER")

		os.Unsetenv("GITHUB_ACTIONS")
		os.Unsetenv("GOTCHA_USE_MOCK")
		os.Unsetenv("GOTCHA_CI_PROVIDER")

		defer func() {
			if oldGithubActions != "" {
				os.Setenv("GITHUB_ACTIONS", oldGithubActions)
			}
			if oldUseMock != "" {
				os.Setenv("GOTCHA_USE_MOCK", oldUseMock)
			}
			if oldCiProvider != "" {
				os.Setenv("GOTCHA_CI_PROVIDER", oldCiProvider)
			}
		}()

		integration := ci.DetectIntegration(logger)
		assert.Nil(t, integration)
	})

	t.Run("mock integration via environment", func(t *testing.T) {
		oldUseMock := os.Getenv("GOTCHA_USE_MOCK")
		os.Setenv("GOTCHA_USE_MOCK", "true")
		defer func() {
			if oldUseMock != "" {
				os.Setenv("GOTCHA_USE_MOCK", oldUseMock)
			} else {
				os.Unsetenv("GOTCHA_USE_MOCK")
			}
		}()

		// Initialize viper to pick up the environment variables
		config.InitEnvironment()

		integration := ci.DetectIntegration(logger)
		assert.NotNil(t, integration, "Integration should not be nil when GOTCHA_USE_MOCK=true")
		if integration != nil {
			assert.Equal(t, "mock", integration.Provider())
		}
	})

	t.Run("manual provider override", func(t *testing.T) {
		oldCiProvider := os.Getenv("GOTCHA_CI_PROVIDER")
		oldUseMock := os.Getenv("GOTCHA_USE_MOCK")

		os.Setenv("GOTCHA_CI_PROVIDER", "mock")
		os.Setenv("GOTCHA_USE_MOCK", "true")

		defer func() {
			if oldCiProvider != "" {
				os.Setenv("GOTCHA_CI_PROVIDER", oldCiProvider)
			} else {
				os.Unsetenv("GOTCHA_CI_PROVIDER")
			}
			if oldUseMock != "" {
				os.Setenv("GOTCHA_USE_MOCK", oldUseMock)
			} else {
				os.Unsetenv("GOTCHA_USE_MOCK")
			}
		}()

		// Initialize viper to pick up the environment variables
		config.InitEnvironment()

		integration := ci.DetectIntegration(logger)
		assert.NotNil(t, integration)
		if integration != nil {
			assert.Equal(t, "mock", integration.Provider())
		}
	})
}

func TestGetIntegration(t *testing.T) {
	logger := log.New(nil)

	t.Run("get mock integration directly", func(t *testing.T) {
		integration := ci.GetIntegration("mock", logger)
		assert.NotNil(t, integration, "Should be able to get mock integration directly")
		if integration != nil {
			assert.Equal(t, "mock", integration.Provider())
			// Verify it's available when GOTCHA_USE_MOCK is set
			oldUseMock := os.Getenv("GOTCHA_USE_MOCK")
			os.Setenv("GOTCHA_USE_MOCK", "true")
			assert.True(t, integration.IsAvailable(), "Mock integration should be available when GOTCHA_USE_MOCK=true")
			if oldUseMock != "" {
				os.Setenv("GOTCHA_USE_MOCK", oldUseMock)
			} else {
				os.Unsetenv("GOTCHA_USE_MOCK")
			}
		}
	})

	t.Run("get unknown integration", func(t *testing.T) {
		integration := ci.GetIntegration("unknown", logger)
		assert.Nil(t, integration)
	})
}

func TestRegisterIntegration(t *testing.T) {
	logger := log.New(nil)

	// Create a custom test integration
	testProvider := "test-custom"

	// Register it
	ci.RegisterIntegration(testProvider, func(l *log.Logger) ci.Integration {
		return mock.NewMockIntegration(l)
	})

	// Try to get it
	integration := ci.GetIntegration(testProvider, logger)
	assert.NotNil(t, integration)
}

func TestGetSupportedProviders(t *testing.T) {
	providers := ci.GetSupportedProviders()
	assert.NotEmpty(t, providers)

	// At minimum, we should have mock and github registered
	hasMock := false
	hasGitHub := false

	for _, p := range providers {
		if p == "mock" {
			hasMock = true
		}
		if p == ci.GitHub {
			hasGitHub = true
		}
	}

	assert.True(t, hasMock, "mock integration should be registered")
	assert.True(t, hasGitHub, "github integration should be registered")
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

		assert.False(t, ci.IsCI())
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

		assert.True(t, ci.IsCI())
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

	// Detect integration
	integration := ci.DetectIntegration(logger)
	assert.NotNil(t, integration)
	assert.Equal(t, "mock", integration.Provider())

	// Get context
	ctx, err := integration.DetectContext()
	assert.NoError(t, err)
	assert.NotNil(t, ctx)

	// Create comment manager
	cm := integration.CreateCommentManager(ctx, logger)
	assert.NotNil(t, cm)

	// Test posting a comment
	err = cm.PostOrUpdateComment(context.Background(), ctx, "Test comment from integration test")
	assert.NoError(t, err)

	// Get job summary writer
	jsw := integration.GetJobSummaryWriter()
	assert.NotNil(t, jsw)

	// Write a summary
	path, err := jsw.WriteJobSummary("# Integration Test Summary")
	assert.NoError(t, err)
	assert.NotEmpty(t, path)

	// Verify we can cast to mock integration and check internal state
	if mockIntegration, ok := integration.(*mock.MockIntegration); ok {
		comments := mockIntegration.GetComments()
		assert.NotEmpty(t, comments)

		summaries := mockIntegration.GetWrittenSummaries()
		assert.NotEmpty(t, summaries)
	}
}
