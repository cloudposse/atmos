package cmd

import (
	"context"
	"fmt"
	"os"
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
	result := shouldPostCommentWithOS(strategy, summary, runtime.GOOS)
	// Debug logging to trace decision
	if globalLogger != nil {
		globalLogger.Debug("shouldPostComment decision",
			"strategy", strategy,
			"os", runtime.GOOS,
			"result", result)
	}
	return result
}

// shouldPostCommentWithOS determines if a comment should be posted based on strategy and OS.
func shouldPostCommentWithOS(strategy string, summary *types.TestSummary, goos string) bool {
	// Normalize strategy (remove dashes for alternative forms)
	normalizedStrategy := strings.ReplaceAll(strategy, "-", "")

	switch normalizedStrategy {
	case "always":
		return true
	case "never", "off", "":
		return false
	case "onfailure":
		return len(summary.Failed) > 0
	case "onskip":
		return len(summary.Skipped) > 0
	case "adaptive":
		// Adaptive: behavior depends on OS
		// On Linux: always post
		// On other OS: only post if there are failures or skips
		if goos == "linux" {
			return true
		}
		return len(summary.Failed) > 0 || len(summary.Skipped) > 0
	default:
		// Check if strategy matches the OS name (linux, darwin, windows, etc.)
		if normalizedStrategy == goos {
			return true
		}
		// For unrecognized strategies, default to never
		return false
	}
}

// postGitHubComment posts a comment to GitHub PR if conditions are met.
func postGitHubComment(summary *types.TestSummary, cmd *cobra.Command, logger *log.Logger) error {
	// Ensure GitHub token is available via viper (for the CI provider to use)
	_ = viper.BindPFlag("github-token", cmd.Flags().Lookup("github-token"))
	_ = viper.BindEnv("github-token", "GITHUB_TOKEN")

	// Get the comment UUID
	_ = viper.BindPFlag("comment-uuid", cmd.Flags().Lookup("comment-uuid"))
	_ = viper.BindEnv("comment-uuid", "GOTCHA_COMMENT_UUID", "COMMENT_UUID")
	commentUUID := viper.GetString("comment-uuid")

	// Comment UUID is required for posting
	if commentUUID == "" {
		logger.Error("Comment UUID is required but not set",
			"GOTCHA_COMMENT_UUID", os.Getenv("GOTCHA_COMMENT_UUID"),
			"COMMENT_UUID", os.Getenv("COMMENT_UUID"))
		return fmt.Errorf("%w", ErrCommentUUIDRequired)
	}

	logger.Debug("Comment UUID found", "uuid", commentUUID)

	// Get the CI provider
	provider := ci.DetectIntegration(logger)
	if provider == nil {
		logger.Warn("No CI provider detected",
			"CI", os.Getenv("CI"),
			"GITHUB_ACTIONS", os.Getenv("GITHUB_ACTIONS"),
			"GITHUB_RUN_ID", os.Getenv("GITHUB_RUN_ID"))
		return nil
	}

	logger.Info("CI provider detected", "provider", provider.Provider())

	// Get context
	ctx, err := provider.DetectContext()
	if err != nil {
		logger.Error("Failed to detect CI context", "error", err)
		return fmt.Errorf("failed to detect CI context: %w", err)
	}

	logger.Debug("CI context detected",
		"repo", ctx.GetRepoName(),
		"pr", ctx.GetPRNumber())

	// Get comment manager
	commentManager := provider.CreateCommentManager(ctx, logger)
	if commentManager == nil {
		logger.Warn("Comment manager not available for this CI provider",
			"provider", provider.Provider())
		return nil
	}

	logger.Debug("Comment manager created successfully")

	// Generate the comment content
	commentContent := markdown.GenerateGitHubComment(summary, commentUUID)

	// Post the comment
	logger.Info("Posting comment to GitHub PR",
		"contentLength", len(commentContent),
		"repo", ctx.GetRepoName(),
		"pr", ctx.GetPRNumber())

	if err := commentManager.PostOrUpdateComment(context.Background(), ctx, commentContent); err != nil {
		logger.Error("Failed to post comment to GitHub",
			"error", err,
			"repo", ctx.GetRepoName(),
			"pr", ctx.GetPRNumber())
		return fmt.Errorf("failed to post comment: %w", err)
	}

	logger.Info("Comment posted successfully",
		"repo", ctx.GetRepoName(),
		"pr", ctx.GetPRNumber())

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
