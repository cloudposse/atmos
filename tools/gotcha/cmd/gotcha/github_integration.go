package cmd

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/tools/gotcha/internal/markdown"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/ci"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// normalizePostingStrategy normalizes the posting strategy value.
func normalizePostingStrategy(strategy string, flagPresent bool) string {
	// If flag wasn't explicitly set, check environment variable
	if !flagPresent {
		// Check environment variables for posting strategy
		_ = viper.BindEnv("GOTCHA_POST_COMMENT", "POST_COMMENT")
		envStrategy := viper.GetString("GOTCHA_POST_COMMENT")
		if envStrategy != "" {
			strategy = envStrategy
		}
	}

	// Trim spaces
	strategy = strings.TrimSpace(strategy)

	// Convert to lowercase for comparison
	lower := strings.ToLower(strategy)

	// Handle boolean aliases
	switch lower {
	case "true", "yes", "1", "on":
		return "always"
	case "false", "no", "0", "off":
		return "never"
	}

	// Normalize named strategies to lowercase
	switch lower {
	case "always", "never", "adaptive", "on-failure", "on-skip":
		return lower
	case "linux", "darwin", "windows":
		return lower
	}

	// Default to "never" if empty (when flag is present but empty)
	if strategy == "" && flagPresent {
		return "never"
	}

	// Default to "on-failure" if still empty (when no flag and no env)
	if strategy == "" {
		return "on-failure"
	}

	// Return the original strategy if it's not a known value
	return strategy
}

// shouldPostComment determines if a comment should be posted based on strategy.
func shouldPostComment(strategy string, summary *types.TestSummary) bool {
	// Delegate to OS-aware function with runtime.GOOS
	return shouldPostCommentWithOS(strategy, summary, runtime.GOOS)
}

// shouldPostCommentWithOS determines if a comment should be posted based on strategy and OS.
func shouldPostCommentWithOS(strategy string, summary *types.TestSummary, goos string) bool {
	switch strategy {
	case "always":
		return true
	case "on-failure":
		// Post if tests failed or if on Windows (which often has flaky tests)
		if len(summary.Failed) > 0 {
			return true
		}
		// Always post on Windows due to potential issues
		if goos == "windows" {
			return true
		}
		return false
	case "off", "never":
		return false
	default:
		// Default to on-failure behavior
		return len(summary.Failed) > 0
	}
}

// postGitHubComment posts a comment to GitHub PR if conditions are met.
func postGitHubComment(summary *types.TestSummary, cmd *cobra.Command, logger *log.Logger) error {
	// Get the comment UUID
	commentUUID, _ := cmd.Flags().GetString("comment-uuid")
	if commentUUID == "" {
		// Try to get from environment
		_ = viper.BindEnv("GOTCHA_COMMENT_UUID", "COMMENT_UUID")
		commentUUID = viper.GetString("GOTCHA_COMMENT_UUID")
	}

	// Comment UUID is required for posting
	if commentUUID == "" {
		return fmt.Errorf("%w", ErrCommentUUIDRequired)
	}

	// Get the CI provider
	provider := ci.DetectIntegration(logger)
	if provider == nil {
		logger.Warn("No CI provider detected")
		return nil
	}

	logger.Debug("CI provider detected", "provider", provider.Provider())

	// Get context
	ctx, err := provider.DetectContext()
	if err != nil {
		return fmt.Errorf("failed to detect CI context: %w", err)
	}

	// Get comment manager
	commentManager := provider.CreateCommentManager(ctx, logger)
	if commentManager == nil {
		logger.Warn("Comment manager not available for this CI provider")
		return nil
	}

	// Generate the comment content
	commentContent := markdown.GenerateGitHubComment(summary, commentUUID)

	// Post the comment
	if err := commentManager.PostOrUpdateComment(context.Background(), ctx, commentContent); err != nil {
		return fmt.Errorf("failed to post comment: %w", err)
	}

	logger.Info("Comment posted successfully")

	// Also write to job summary if supported
	if writer := provider.GetJobSummaryWriter(); writer != nil {
		if path, err := writer.WriteJobSummary(commentContent); err != nil {
			logger.Debug("Failed to write job summary", "error", err)
			// Don't fail the command for this
		} else {
			logger.Debug("Job summary written", "path", path)
		}
	}

	// Log summary information
	total := len(summary.Passed) + len(summary.Failed) + len(summary.Skipped)
	logger.Info("Test summary",
		"passed", len(summary.Passed),
		"failed", len(summary.Failed),
		"skipped", len(summary.Skipped),
		"total", total)

	if coverage := calculateAverageCoverage(summary); coverage >= 0 {
		logger.Info("Coverage", "percentage", fmt.Sprintf("%.1f%%", coverage))
	}

	return nil
}

// calculateAverageCoverage extracts the coverage percentage from the summary.
func calculateAverageCoverage(summary *types.TestSummary) float64 {
	if summary.Coverage == "" {
		return -1
	}

	// Parse coverage string (e.g., "75.5%" -> 75.5)
	var percentage float64
	if _, err := fmt.Sscanf(summary.Coverage, "%f%%", &percentage); err != nil {
		return -1
	}

	return percentage
}
