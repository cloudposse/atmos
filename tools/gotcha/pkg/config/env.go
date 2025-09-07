package config

import (
	"github.com/spf13/viper"
)

// InitEnvironment initializes all environment variable bindings.
// This centralizes environment variable configuration to avoid os.Getenv usage.
func InitEnvironment() {
	// CI environment detection
	viper.BindEnv("ci", "CI")
	viper.BindEnv("github.actions", "GITHUB_ACTIONS")
	viper.BindEnv("github.run.id", "GITHUB_RUN_ID")
	
	// GitHub context
	viper.BindEnv("github.repository", "GITHUB_REPOSITORY")
	viper.BindEnv("github.event.name", "GITHUB_EVENT_NAME")
	viper.BindEnv("github.event.path", "GITHUB_EVENT_PATH")
	viper.BindEnv("github.step.summary", "GITHUB_STEP_SUMMARY")
	
	// Authentication
	viper.BindEnv("github.token", "GITHUB_TOKEN", "GOTCHA_GITHUB_TOKEN")
	
	// Comment configuration
	viper.BindEnv("comment.uuid", "GOTCHA_COMMENT_UUID", "COMMENT_UUID")
	viper.BindEnv("post.comment", "GOTCHA_POST_COMMENT", "POST_COMMENT")
	
	// Mock configuration
	viper.BindEnv("use.mock", "GOTCHA_USE_MOCK")
	
	// Color configuration
	viper.BindEnv("no.color", "NO_COLOR")
	viper.BindEnv("force.color", "FORCE_COLOR")
	viper.BindEnv("term", "TERM")
	viper.BindEnv("colorterm", "COLORTERM")
	
	// Set defaults
	viper.SetDefault("ci", false)
	viper.SetDefault("github.actions", false)
	viper.SetDefault("use.mock", false)
	viper.SetDefault("no.color", false)
	viper.SetDefault("force.color", false)
}

// IsCI returns true if running in a CI environment.
func IsCI() bool {
	return viper.GetBool("ci") || viper.GetBool("github.actions")
}

// IsGitHubActions returns true if running in GitHub Actions.
func IsGitHubActions() bool {
	return viper.GetBool("github.actions")
}

// GetGitHubToken returns the GitHub token if available.
func GetGitHubToken() string {
	return viper.GetString("github.token")
}

// GetCommentUUID returns the comment UUID if available.
func GetCommentUUID() string {
	return viper.GetString("comment.uuid")
}

// UseMock returns true if mock mode is enabled.
func UseMock() bool {
	return viper.GetBool("use.mock")
}

// NoColor returns true if color output should be disabled.
func NoColor() bool {
	return viper.GetBool("no.color")
}

// ForceColor returns true if color output should be forced.
func ForceColor() bool {
	return viper.GetBool("force.color")
}

// GetGitHubRepository returns the GitHub repository name.
func GetGitHubRepository() string {
	return viper.GetString("github.repository")
}

// GetGitHubEventName returns the GitHub event name.
func GetGitHubEventName() string {
	return viper.GetString("github.event.name")
}

// GetGitHubEventPath returns the GitHub event path.
func GetGitHubEventPath() string {
	return viper.GetString("github.event.path")
}

// GetGitHubStepSummary returns the GitHub step summary path.
func GetGitHubStepSummary() string {
	return viper.GetString("github.step.summary")
}