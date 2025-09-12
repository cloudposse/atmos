package mock_test

import (
	"context"
	"fmt"
	"os"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/ci"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/ci/mock"
	_ "github.com/cloudposse/atmos/tools/gotcha/pkg/ci/mock" // Register integration
	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
)

func Example_usingMockIntegration() {
	// Enable mock integration via environment
	os.Setenv("GOTCHA_USE_MOCK", "true")
	defer os.Unsetenv("GOTCHA_USE_MOCK")

	// Initialize viper to pick up the environment variables
	config.InitEnvironment()

	logger := log.New(nil)

	// Auto-detect integration (will find mock when GOTCHA_USE_MOCK=true)
	integration := ci.DetectIntegration(logger)
	if integration == nil {
		fmt.Println("No integration detected")
		return
	}

	fmt.Printf("Detected provider: %s\n", integration.Provider())

	// Get context
	ctx, err := integration.DetectContext()
	if err != nil {
		fmt.Printf("Failed to detect context: %v\n", err)
		return
	}

	fmt.Printf("Repository: %s/%s\n", ctx.GetOwner(), ctx.GetRepo())
	fmt.Printf("PR Number: %d\n", ctx.GetPRNumber())

	// Create comment manager
	commentManager := integration.CreateCommentManager(ctx, logger)

	// Post a comment
	err = commentManager.PostOrUpdateComment(context.Background(), ctx, "Test results: All passed!")
	if err != nil {
		fmt.Printf("Failed to post comment: %v\n", err)
		return
	}

	fmt.Println("Comment posted successfully")

	// Output:
	// Detected provider: mock
	// Repository: mock-owner/mock-repo
	// PR Number: 42
	// Comment posted successfully
}

func Example_configurableMockIntegration() {
	logger := log.New(nil)

	// Create a custom configuration
	config := &mock.MockConfig{
		IsAvailable:         true,
		ContextSupported:    true,
		Owner:               "test-org",
		Repo:                "test-repo",
		PRNumber:            123,
		CommentUUID:         "test-run-456",
		Comments:            make(map[string]string),
		JobSummarySupported: true,
		JobSummaryPath:      "/tmp/test-summary.md",
		WrittenSummaries:    []string{},
	}

	// Create mock integration with custom config
	integration := mock.NewMockIntegrationWithConfig(logger, config)

	// Use it for testing
	ctx, _ := integration.DetectContext()
	cm := integration.CreateCommentManager(ctx, logger)

	// Post a comment
	cm.PostOrUpdateComment(context.Background(), ctx, "Custom test comment")

	// Verify the comment was stored
	comments := integration.GetComments()
	fmt.Printf("Stored comments: %d\n", len(comments))
	fmt.Printf("Comment content: %s\n", comments["test-run-456"])

	// Output:
	// Stored comments: 1
	// Comment content: Custom test comment
}

func Example_testingWithFailures() {
	logger := log.New(nil)

	// Configure provider to fail certain operations
	config := &mock.MockConfig{
		IsAvailable:       true,
		ContextSupported:  true,
		Owner:             "test-org",
		Repo:              "test-repo",
		PRNumber:          789,
		ShouldFailComment: true,
		CommentError:      fmt.Errorf("simulated API failure"),
		Comments:          make(map[string]string),
	}

	integration := mock.NewMockIntegrationWithConfig(logger, config)

	ctx, _ := integration.DetectContext()
	cm := integration.CreateCommentManager(ctx, logger)

	// Try to post a comment (will fail)
	err := cm.PostOrUpdateComment(context.Background(), ctx, "This will fail")
	if err != nil {
		fmt.Printf("Expected failure: %v\n", err)
	}

	// Output:
	// Expected failure: simulated API failure
}
