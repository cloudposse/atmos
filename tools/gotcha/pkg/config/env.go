package config

import (
	"github.com/spf13/viper"
)

// InitEnvironment initializes all environment variable bindings.
// This centralizes environment variable configuration to avoid os.Getenv usage.
// GOTCHA_ prefixed variables take precedence over standard names.
func InitEnvironment() {
	// Runtime environment detection - These ONLY read from env vars, not config files
	// Use "runtime.*" keys for actual environment detection
	_ = viper.BindEnv("runtime.ci", "CI")
	_ = viper.BindEnv("runtime.github.actions", "GITHUB_ACTIONS")
	_ = viper.BindEnv("runtime.github.run.id", "GITHUB_RUN_ID")

	// Configuration settings - These can be set in config files OR env vars
	// Use regular keys for feature configuration
	_ = viper.BindEnv("ci", "GOTCHA_CI")
	_ = viper.BindEnv("github.actions", "GOTCHA_GITHUB_ACTIONS")
	_ = viper.BindEnv("github.run.id", "GOTCHA_GITHUB_RUN_ID")
	_ = viper.BindEnv("ci.provider", "GOTCHA_CI_PROVIDER")

	// GitHub context - GOTCHA_ variants first for override capability
	_ = viper.BindEnv("github.repository", "GOTCHA_GITHUB_REPOSITORY", "GITHUB_REPOSITORY")
	_ = viper.BindEnv("github.event.name", "GOTCHA_GITHUB_EVENT_NAME", "GITHUB_EVENT_NAME")
	_ = viper.BindEnv("github.event.path", "GOTCHA_GITHUB_EVENT_PATH", "GITHUB_EVENT_PATH")
	_ = viper.BindEnv("github.step.summary", "GOTCHA_GITHUB_STEP_SUMMARY", "GITHUB_STEP_SUMMARY")

	// Authentication - GOTCHA_ takes precedence
	_ = viper.BindEnv("github.token", "GOTCHA_GITHUB_TOKEN", "GITHUB_TOKEN")

	// Comment configuration - GOTCHA_ takes precedence
	_ = viper.BindEnv("comment.uuid", "GOTCHA_COMMENT_UUID", "COMMENT_UUID")
	_ = viper.BindEnv("post.comment", "GOTCHA_POST_COMMENT", "POST_COMMENT")

	// Mock configuration
	_ = viper.BindEnv("use.mock", "GOTCHA_USE_MOCK")

	// TTY configuration
	_ = viper.BindEnv("force.tty", "GOTCHA_FORCE_TTY", "FORCE_TTY")
	_ = viper.BindEnv("force.no.tty", "GOTCHA_FORCE_NO_TTY", "FORCE_NO_TTY")

	// Color configuration - standard names (no GOTCHA_ prefix typically)
	_ = viper.BindEnv("no.color", "NO_COLOR")
	_ = viper.BindEnv("force.color", "FORCE_COLOR")
	_ = viper.BindEnv("term", "TERM")
	_ = viper.BindEnv("colorterm", "COLORTERM")

	// Output configuration
	_ = viper.BindEnv("output", "GOTCHA_OUTPUT")
	_ = viper.BindEnv("show", "GOTCHA_SHOW")

	// Additional CI providers for runtime detection
	_ = viper.BindEnv("runtime.continuous.integration", "CONTINUOUS_INTEGRATION")
	_ = viper.BindEnv("runtime.build.number", "BUILD_NUMBER")
	_ = viper.BindEnv("runtime.jenkins.url", "JENKINS_URL")
	_ = viper.BindEnv("runtime.travis", "TRAVIS")
	_ = viper.BindEnv("runtime.circleci", "CIRCLECI")

	// Debug and test configuration
	_ = viper.BindEnv("debug.file", "GOTCHA_DEBUG_FILE")
	_ = viper.BindEnv("test.mode", "GOTCHA_TEST_MODE")
	_ = viper.BindEnv("force.tui", "GOTCHA_FORCE_TUI")
	_ = viper.BindEnv("split.streams", "GOTCHA_SPLIT_STREAMS")

	// Terminal configuration
	_ = viper.BindEnv("columns", "COLUMNS")

	// Set defaults
	viper.SetDefault("ci", false)
	viper.SetDefault("github.actions", false)
	viper.SetDefault("use.mock", false)
	viper.SetDefault("no.color", false)
	viper.SetDefault("force.color", false)
	viper.SetDefault("force.tty", false)
	viper.SetDefault("force.no.tty", false)
}

// IsCI returns true if ACTUALLY running in a CI environment (runtime check).
// This checks actual environment variables, not config settings.
func IsCI() bool {
	// Check runtime.* keys which are ONLY bound to env vars
	return viper.GetString("runtime.ci") != "" ||
		viper.GetString("runtime.github.actions") != "" ||
		viper.GetString("runtime.continuous.integration") != "" ||
		viper.GetString("runtime.build.number") != "" ||
		viper.GetString("runtime.jenkins.url") != "" ||
		viper.GetString("runtime.travis") != "" ||
		viper.GetString("runtime.circleci") != ""
}

// IsCIEnabled returns true if CI features are enabled in configuration.
// This checks config settings, which can come from config files or GOTCHA_* env vars.
func IsCIEnabled() bool {
	return viper.GetBool("ci")
}

// IsGitHubActions returns true if ACTUALLY running in GitHub Actions (runtime check).
func IsGitHubActions() bool {
	// Check runtime key which is ONLY bound to GITHUB_ACTIONS env var
	return viper.GetString("runtime.github.actions") == "true"
}

// IsGitHubActionsEnabled returns true if GitHub Actions features are enabled in config.
func IsGitHubActionsEnabled() bool {
	// This checks the config setting, which can come from files or GOTCHA_GITHUB_ACTIONS
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

// GetDebugFile returns the debug file path if configured.
func GetDebugFile() string {
	return viper.GetString("debug.file")
}

// GetColumns returns the COLUMNS environment variable value.
func GetColumns() string {
	return viper.GetString("columns")
}

// IsSplitStreams returns true if streams should be split.
func IsSplitStreams() bool {
	return viper.GetBool("split.streams")
}
