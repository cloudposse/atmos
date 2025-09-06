package vcs_test

import (
	"context"
	"os"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/vcs"
	_ "github.com/cloudposse/atmos/tools/gotcha/pkg/vcs/github" // Register GitHub
	_ "github.com/cloudposse/atmos/tools/gotcha/pkg/vcs/mock"   // Register Mock
	"github.com/stretchr/testify/assert"
)

// TestVCSAbstractionIntegration tests the complete VCS abstraction flow
func TestVCSAbstractionIntegration(t *testing.T) {
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
		
		// Detect provider
		provider := vcs.DetectProvider(logger)
		assert.NotNil(t, provider)
		assert.Equal(t, vcs.Platform("mock"), provider.GetPlatform())
		
		// Get context
		ctx, err := provider.DetectContext()
		assert.NoError(t, err)
		assert.NotNil(t, ctx)
		assert.True(t, ctx.IsSupported())
		
		// Test comment posting
		cm := provider.CreateCommentManager(ctx, logger)
		assert.NotNil(t, cm)
		
		testComment := "# Test Results\n\n✅ All tests passed!"
		err = cm.PostOrUpdateComment(context.Background(), ctx, testComment)
		assert.NoError(t, err)
		
		// Test job summary (if supported)
		jsw := provider.GetJobSummaryWriter()
		if jsw != nil {
			assert.True(t, jsw.IsJobSummarySupported())
			path, err := jsw.WriteJobSummary(testComment)
			assert.NoError(t, err)
			assert.NotEmpty(t, path)
		}
	})
	
	// Test provider detection without any environment
	t.Run("no provider available", func(t *testing.T) {
		// Clear all VCS-related environment variables
		oldVars := map[string]string{
			"GOTCHA_USE_MOCK":     os.Getenv("GOTCHA_USE_MOCK"),
			"GOTCHA_VCS_PLATFORM": os.Getenv("GOTCHA_VCS_PLATFORM"),
			"GITHUB_ACTIONS":      os.Getenv("GITHUB_ACTIONS"),
		}
		
		os.Unsetenv("GOTCHA_USE_MOCK")
		os.Unsetenv("GOTCHA_VCS_PLATFORM")
		os.Unsetenv("GITHUB_ACTIONS")
		
		defer func() {
			for k, v := range oldVars {
				if v != "" {
					os.Setenv(k, v)
				}
			}
		}()
		
		provider := vcs.DetectProvider(logger)
		assert.Nil(t, provider)
	})
}

// TestVCSCommentSizing tests that comments handle size limits correctly
func TestVCSCommentSizing(t *testing.T) {
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
	
	provider := vcs.DetectProvider(logger)
	assert.NotNil(t, provider)
	
	ctx, err := provider.DetectContext()
	assert.NoError(t, err)
	
	cm := provider.CreateCommentManager(ctx, logger)
	
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
	provider := vcs.DetectProvider(logger)
	if provider == nil {
		t.Skip("No VCS provider available")
	}
	
	ctx, err := provider.DetectContext()
	if err != nil || !ctx.IsSupported() {
		t.Skip("VCS context not supported")
	}
	
	cm := provider.CreateCommentManager(ctx, logger)
	
	// Create markdown comment (simplified version)
	comment := "# Test Results\n\n"
	comment += "## Summary\n"
	comment += "- ✅ Passed: " + string(rune(len(summary.Passed))) + "\n"
	comment += "- ❌ Failed: " + string(rune(len(summary.Failed))) + "\n"
	
	err = cm.PostOrUpdateComment(context.Background(), ctx, comment)
	assert.NoError(t, err)
}