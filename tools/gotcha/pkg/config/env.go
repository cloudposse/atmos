package config

import (
	"github.com/spf13/viper"
)

// InitEnvironment initializes all environment variable bindings.
// This centralizes environment variable configuration to avoid os.Getenv usage.
// GOTCHA_ prefixed variables take precedence over standard names.
func InitEnvironment() {
	// CI environment detection - GOTCHA_ variants take precedence
	viper.BindEnv("ci", "GOTCHA_CI", "CI")
	viper.BindEnv("github.actions", "GOTCHA_GITHUB_ACTIONS", "GITHUB_ACTIONS")
	viper.BindEnv("github.run.id", "GOTCHA_GITHUB_RUN_ID", "GITHUB_RUN_ID")
	viper.BindEnv("ci.provider", "GOTCHA_CI_PROVIDER")

	// GitHub context - GOTCHA_ variants first for override capability
	viper.BindEnv("github.repository", "GOTCHA_GITHUB_REPOSITORY", "GITHUB_REPOSITORY")
	viper.BindEnv("github.event.name", "GOTCHA_GITHUB_EVENT_NAME", "GITHUB_EVENT_NAME")
	viper.BindEnv("github.event.path", "GOTCHA_GITHUB_EVENT_PATH", "GITHUB_EVENT_PATH")
	viper.BindEnv("github.step.summary", "GOTCHA_GITHUB_STEP_SUMMARY", "GITHUB_STEP_SUMMARY")

	// Authentication - GOTCHA_ takes precedence
	viper.BindEnv("github.token", "GOTCHA_GITHUB_TOKEN", "GITHUB_TOKEN")

	// Comment configuration - GOTCHA_ takes precedence
	viper.BindEnv("comment.uuid", "GOTCHA_COMMENT_UUID", "COMMENT_UUID")
	viper.BindEnv("post.comment", "GOTCHA_POST_COMMENT", "POST_COMMENT")

	// Mock configuration
	viper.BindEnv("use.mock", "GOTCHA_USE_MOCK")

	// TTY configuration
	viper.BindEnv("force.tty", "GOTCHA_FORCE_TTY", "FORCE_TTY")
	viper.BindEnv("force.no.tty", "GOTCHA_FORCE_NO_TTY", "FORCE_NO_TTY")

	// Color configuration - standard names (no GOTCHA_ prefix typically)
	viper.BindEnv("no.color", "NO_COLOR")
	viper.BindEnv("force.color", "FORCE_COLOR")
	viper.BindEnv("term", "TERM")
	viper.BindEnv("colorterm", "COLORTERM")

	// Output configuration
	viper.BindEnv("output", "GOTCHA_OUTPUT")
	viper.BindEnv("show", "GOTCHA_SHOW")

	// Additional CI providers for detection
	viper.BindEnv("continuous.integration", "CONTINUOUS_INTEGRATION")
	viper.BindEnv("build.number", "BUILD_NUMBER")
	viper.BindEnv("jenkins.url", "JENKINS_URL")
	viper.BindEnv("travis", "TRAVIS")
	viper.BindEnv("circleci", "CIRCLECI")

	// Set defaults
	viper.SetDefault("ci", false)
	viper.SetDefault("github.actions", false)
	viper.SetDefault("use.mock", false)
	viper.SetDefault("no.color", false)
	viper.SetDefault("force.color", false)
	viper.SetDefault("force.tty", false)
	viper.SetDefault("force.no.tty", false)
}

// IsCI returns true if running in a CI environment.
func IsCI() bool {
	// Check multiple CI indicators
	return viper.GetBool("ci") ||
		viper.GetBool("github.actions") ||
		viper.GetString("continuous.integration") != "" ||
		viper.GetString("build.number") != "" ||
		viper.GetString("jenkins.url") != "" ||
		viper.GetBool("travis") ||
		viper.GetBool("circleci")
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

// ForceTTY returns true if TTY should be forced on.
func ForceTTY() bool {
	return viper.GetBool("force.tty")
}

// ForceNoTTY returns true if TTY should be forced off.
func ForceNoTTY() bool {
	return viper.GetBool("force.no.tty")
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

// GetCIProvider returns the CI provider if explicitly set.
func GetCIProvider() string {
	return viper.GetString("ci.provider")
}

// GetShowFilter returns the show filter configuration.
func GetShowFilter() string {
	return viper.GetString("show")
}

// GetOutput returns the output configuration.
func GetOutput() string {
	return viper.GetString("output")
}
