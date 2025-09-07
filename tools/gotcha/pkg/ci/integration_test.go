package ci_test

import (
	"context"
	"os"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/ci"
	_ "github.com/cloudposse/atmos/tools/gotcha/pkg/ci/github" // Register GitHub
	_ "github.com/cloudposse/atmos/tools/gotcha/pkg/ci/mock"   // Register Mock
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/stretchr/testify/assert"
)

// TestCIAbstractionIntegration tests the complete CI abstraction flow
func TestCIAbstractionIntegration(t *testing.T) {
	logger := log.New(nil)
	logger.SetLevel(log.DebugLevel)

	// Test with mock provider
	t.Run("mock provider flow", func(t *testing.T) {
		// Enable mock provider
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
		assert.True(t, ctx.IsSupported())

		// Test comment posting
		cm := integration.CreateCommentManager(ctx, logger)
		assert.NotNil(t, cm)

		testComment := "# Test Results\n\n✅ All tests passed!"
		err = cm.PostOrUpdateComment(context.Background(), ctx, testComment)
		assert.NoError(t, err)

		// Test job summary (if supported)
		jsw := integration.GetJobSummaryWriter()
		if jsw != nil {
			assert.True(t, jsw.IsJobSummarySupported())
			path, err := jsw.WriteJobSummary(testComment)
			assert.NoError(t, err)
			assert.NotEmpty(t, path)
		}
	})

	// Test provider detection without any environment
	t.Run("no provider available", func(t *testing.T) {
		// Clear all CI-related environment variables
		oldVars := map[string]string{
			"GOTCHA_USE_MOCK":    os.Getenv("GOTCHA_USE_MOCK"),
			"GOTCHA_CI_PROVIDER": os.Getenv("GOTCHA_CI_PROVIDER"),
			"GITHUB_ACTIONS":     os.Getenv("GITHUB_ACTIONS"),
		}

		os.Unsetenv("GOTCHA_USE_MOCK")
		os.Unsetenv("GOTCHA_CI_PROVIDER")
		os.Unsetenv("GITHUB_ACTIONS")

		defer func() {
			for k, v := range oldVars {
				if v != "" {
					os.Setenv(k, v)
				}
			}
		}()

		integration := ci.DetectIntegration(logger)
		assert.Nil(t, integration)
	})
}

// TestCICommentSizing tests that comments handle size limits correctly
func TestCICommentSizing(t *testing.T) {
	logger := log.New(nil)

	// Enable mock provider
	oldUseMock := os.Getenv("GOTCHA_USE_MOCK")
	os.Setenv("GOTCHA_USE_MOCK", "true")
	defer func() {
		if oldUseMock != "" {
			os.Setenv("GOTCHA_USE_MOCK", oldUseMock)
		} else {
			os.Unsetenv("GOTCHA_USE_MOCK")
		}
	}()

	integration := ci.DetectIntegration(logger)
	assert.NotNil(t, integration)

	ctx, err := integration.DetectContext()
	assert.NoError(t, err)

	cm := integration.CreateCommentManager(ctx, logger)

	// Create a large comment (mock provider should handle any size)
	largeComment := "# Large Test Report\n\n"
	for i := 0; i < 1000; i++ {
		largeComment += "This is test line number " + string(rune(i)) + " with some content.\n"
	}

	err = cm.PostOrUpdateComment(context.Background(), ctx, largeComment)
	assert.NoError(t, err)
}

// TestSimulateGotchaCommentPosting simulates how gotcha would post a comment
func TestSimulateGotchaCommentPosting(t *testing.T) {
	logger := log.New(nil)

	// Enable mock provider
	os.Setenv("GOTCHA_USE_MOCK", "true")
	defer os.Unsetenv("GOTCHA_USE_MOCK")

	// Simulate test summary
	summary := &types.TestSummary{
		Passed: []types.TestResult{
			{Package: "pkg/foo", Test: "TestFoo", Status: "pass", Duration: 0.1},
		},
		Failed: []types.TestResult{
			{Package: "pkg/bar", Test: "TestBar", Status: "fail", Duration: 0.2},
		},
		TotalElapsedTime: 0.3,
	}

	// This simulates what postGitHubComment does
	integration := ci.DetectIntegration(logger)
	if integration == nil {
		t.Skip("No CI integration available")
	}

	ctx, err := integration.DetectContext()
	if err != nil || !ctx.IsSupported() {
		t.Skip("CI context not supported")
	}

	cm := integration.CreateCommentManager(ctx, logger)

	// Create markdown comment (simplified version)
	comment := "# Test Results\n\n"
	comment += "## Summary\n"
	comment += "- ✅ Passed: " + string(rune(len(summary.Passed))) + "\n"
	comment += "- ❌ Failed: " + string(rune(len(summary.Failed))) + "\n"

	err = cm.PostOrUpdateComment(context.Background(), ctx, comment)
	assert.NoError(t, err)
}
