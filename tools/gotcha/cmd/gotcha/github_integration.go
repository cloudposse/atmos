package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/tools/gotcha/internal/markdown"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/ci"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
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
		return FlagNever
	}

	// Normalize named strategies to lowercase
	switch lower {
	case "always", FlagNever, "adaptive", "on-failure", "on-skip":
		return lower
	case "linux", "darwin", "windows":
		return lower
	}

	// Default to "never" if empty (when flag is present but empty)
	if strategy == "" && flagPresent {
		return FlagNever
	}

	// Default to "on-failure" if still empty (when no flag and no env)
	if strategy == "" {
		return "on-failure"
	}

	// Return the original strategy if it's not a known value
	return strategy
}

// checkForToolsDirectory checks if the parent directory is named "tools".
func checkForToolsDirectory(dir string) (string, bool) {
	parent := filepath.Dir(dir)
	parentName := filepath.Base(parent)
	currentName := filepath.Base(dir)

	if parentName == "tools" {
		if globalLogger != nil {
			globalLogger.Debug("Found tools directory", "parent", parent, "tool", currentName)
		}
		return currentName, true
	}
	return "", false
}

// checkForGitRoot checks if the directory contains a .git directory.
func checkForGitRoot(dir string) (string, bool) {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		repoName := filepath.Base(dir)
		if globalLogger != nil {
			globalLogger.Debug("Found repository root", "dir", dir, "repo", repoName)
		}
		return repoName, true
	}
	return "", false
}

// detectProjectContext auto-detects the project context based on the current working directory.
func detectProjectContext() string {
	cwd, err := os.Getwd()
	if err != nil {
		if globalLogger != nil {
			globalLogger.Debug("Failed to get working directory", "error", err)
		}
		return ""
	}

	if globalLogger != nil {
		globalLogger.Debug("Detecting project context", "cwd", cwd)
	}

	// Walk up the directory tree
	dir := cwd
	for {
		// Check for tools directory
		if name, found := checkForToolsDirectory(dir); found {
			return name
		}

		// Check for git root
		if name, found := checkForGitRoot(dir); found {
			return name
		}

		// Stop if we've reached the filesystem root
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Default to basename of current directory
	baseName := filepath.Base(cwd)
	if globalLogger != nil {
		globalLogger.Debug("Using current directory name as fallback", "name", baseName)
	}
	return baseName
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
	case FlagNever, "off", "":
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

// getCommentUUID retrieves and validates the comment UUID.
func getCommentUUID(cmd *cobra.Command, logger *log.Logger) (string, error) {
	_ = viper.BindPFlag(FlagCommentUUID, cmd.Flags().Lookup(FlagCommentUUID))
	_ = viper.BindEnv(FlagCommentUUID, "GOTCHA_COMMENT_UUID", "COMMENT_UUID")
	commentUUID := viper.GetString(FlagCommentUUID)

	if commentUUID == "" {
		logger.Error("Comment UUID is required but not set",
			"GOTCHA_COMMENT_UUID", config.GetCommentUUID(),
			"COMMENT_UUID", viper.GetString("comment.uuid"))
		return "", fmt.Errorf("%w", ErrCommentUUIDRequired)
	}

	logger.Debug("Comment UUID found", "uuid", commentUUID)
	return commentUUID, nil
}

// getDiscriminator builds the discriminator from project context and job discriminator.
func getDiscriminator(logger *log.Logger) string {
	_ = viper.BindEnv("job-discriminator", "GOTCHA_JOB_DISCRIMINATOR", "JOB_DISCRIMINATOR")
	jobDiscriminator := viper.GetString("job-discriminator")
	if jobDiscriminator != "" {
		logger.Debug("Job discriminator found", "discriminator", jobDiscriminator)
	}

	_ = viper.BindEnv("project-context", "GOTCHA_PROJECT_CONTEXT", "PROJECT_CONTEXT")
	projectContext := viper.GetString("project-context")

	if projectContext == "" {
		projectContext = detectProjectContext()
	}

	if projectContext != "" {
		logger.Debug("Project context determined", "context", projectContext)
	}

	var discriminator string
	switch {
	case projectContext != "" && jobDiscriminator != "":
		discriminator = fmt.Sprintf("%s/%s", projectContext, jobDiscriminator)
	case projectContext != "":
		discriminator = projectContext
	case jobDiscriminator != "":
		discriminator = jobDiscriminator
	}

	if discriminator != "" {
		logger.Debug("Using compound discriminator", "discriminator", discriminator)
	}

	return discriminator
}

// postGitHubComment posts a comment to GitHub PR if conditions are met.
// setupGitHubToken ensures GitHub token is available via viper.
func setupGitHubToken(cmd *cobra.Command) {
	_ = viper.BindPFlag(FlagGithubToken, cmd.Flags().Lookup(FlagGithubToken))
	_ = viper.BindEnv(FlagGithubToken, "GITHUB_TOKEN")
}

// detectCIProvider detects and returns the CI provider, or nil if none found.
func detectCIProvider(logger *log.Logger) ci.Integration {
	provider := ci.DetectIntegration(logger)
	if provider == nil {
		logger.Warn("No CI provider detected",
			"CI", viper.GetString("ci"),
			"GITHUB_ACTIONS", viper.GetString("github.actions"),
			"GITHUB_RUN_ID", viper.GetString("github.run.id"))
	}
	return provider
}

// getCIContextAndManager gets the CI context and comment manager from the provider.
func getCIContextAndManager(provider ci.Integration, logger *log.Logger) (ci.Context, ci.CommentManager, error) {
	ctx, err := provider.DetectContext()
	if err != nil {
		logger.Error("Failed to detect CI context", "error", err)
		return nil, nil, fmt.Errorf("failed to detect CI context: %w", err)
	}

	logger.Debug("CI context detected",
		FlagRepo, ctx.GetRepo(),
		FlagPR, ctx.GetPRNumber())

	commentManager := provider.CreateCommentManager(ctx, logger)
	if commentManager == nil {
		logger.Warn("Comment manager not available for this CI provider",
			"provider", provider.Provider())
		return ctx, nil, nil
	}

	logger.Debug("Comment manager created successfully")
	return ctx, commentManager, nil
}

// postComment posts the comment to the PR and writes job summary if supported.
func postComment(provider ci.Integration, ctx ci.Context, commentManager ci.CommentManager, 
	commentContent string, logger *log.Logger) error {
	
	logger.Info("Posting comment to GitHub PR",
		"contentLength", len(commentContent),
		FlagRepo, ctx.GetRepo(),
		FlagPR, ctx.GetPRNumber())

	if err := commentManager.PostOrUpdateComment(context.Background(), ctx, commentContent); err != nil {
		logger.Error("Failed to post comment to GitHub",
			"error", err,
			"repo", ctx.GetRepo(),
			"pr", ctx.GetPRNumber())
		return fmt.Errorf("failed to post comment: %w", err)
	}

	logger.Info("Comment posted successfully",
		FlagRepo, ctx.GetRepo(),
		FlagPR, ctx.GetPRNumber())

	// Also write to job summary if supported
	if writer := provider.GetJobSummaryWriter(); writer != nil {
		if path, err := writer.WriteJobSummary(commentContent); err != nil {
			logger.Debug("Failed to write job summary", "error", err)
			// Don't fail the command for this
		} else {
			logger.Debug("Job summary written", "path", path)
		}
	}

	return nil
}

// logTestSummary logs the test summary information.
func logTestSummary(summary *types.TestSummary, logger *log.Logger) {
	total := len(summary.Passed) + len(summary.Failed) + len(summary.Skipped)
	logger.Info("Test summary",
		"passed", len(summary.Passed),
		"failed", len(summary.Failed),
		"skipped", len(summary.Skipped),
		"total", total)

	if coverage := calculateAverageCoverage(summary); coverage >= 0 {
		logger.Info("Coverage", "percentage", fmt.Sprintf("%.1f%%", coverage))
	}
}

func postGitHubComment(summary *types.TestSummary, cmd *cobra.Command, logger *log.Logger) error {
	// Setup GitHub token
	setupGitHubToken(cmd)

	// Get comment UUID and discriminator
	commentUUID, err := getCommentUUID(cmd, logger)
	if err != nil {
		return err
	}
	discriminator := getDiscriminator(logger)

	// Detect CI provider
	provider := detectCIProvider(logger)
	if provider == nil {
		return nil
	}
	logger.Info("CI provider detected", "provider", provider.Provider())

	// Get CI context and comment manager
	ctx, commentManager, err := getCIContextAndManager(provider, logger)
	if err != nil {
		return err
	}
	if commentManager == nil {
		return nil
	}

	// Generate and post comment
	commentContent := markdown.GenerateAdaptiveComment(summary, commentUUID, discriminator)
	if err := postComment(provider, ctx, commentManager, commentContent, logger); err != nil {
		return err
	}

	// Log summary
	logTestSummary(summary, logger)

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
