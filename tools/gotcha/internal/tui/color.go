package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/viper"
)

// detectColorProfile determines the appropriate color profile based on environment.
func detectColorProfile() termenv.Profile {
	// Bind environment variables using viper
	_ = viper.BindEnv("NO_COLOR")
	_ = viper.BindEnv("FORCE_COLOR")
	_ = viper.BindEnv("TERM")
	_ = viper.BindEnv("COLORTERM")
	_ = viper.BindEnv("GITHUB_ACTIONS")
	_ = viper.BindEnv("CI")
	_ = viper.BindEnv("CONTINUOUS_INTEGRATION")
	_ = viper.BindEnv("BUILD_NUMBER")
	_ = viper.BindEnv("JENKINS_URL")
	_ = viper.BindEnv("TRAVIS")
	_ = viper.BindEnv("CIRCLECI")
	_ = viper.BindEnv("GITLAB_CI")
	_ = viper.BindEnv("APPVEYOR")
	_ = viper.BindEnv("BUILDKITE")
	_ = viper.BindEnv("DRONE")

	// Check if colors are explicitly disabled
	if viper.GetString("NO_COLOR") != "" {
		return termenv.Ascii
	}

	// Check if colors are explicitly enabled
	if viper.GetString("FORCE_COLOR") != "" {
		// FORCE_COLOR can have values like "1", "2", "3" for different color levels
		switch viper.GetString("FORCE_COLOR") {
		case "2":
			return termenv.ANSI256
		case "3":
			return termenv.TrueColor
		default:
			return termenv.ANSI
		}
	}

	// Special handling for GitHub Actions and other CI environments
	if IsGitHubActions() {
		// GitHub Actions supports ANSI colors
		// Check if TERM suggests 256 color support
		if term := viper.GetString("TERM"); term == "xterm-256color" || term == "screen-256color" {
			return termenv.ANSI256
		}
		return termenv.ANSI
	}

	// Generic CI detection - most CI environments support at least ANSI colors
	if IsCI() {
		return termenv.ANSI
	}

	// For local development, check for explicit color support indicators
	// Check for truecolor support
	if colorterm := viper.GetString("COLORTERM"); colorterm == "truecolor" || colorterm == "24bit" {
		return termenv.TrueColor
	}

	// Check TERM for 256 color support
	if term := viper.GetString("TERM"); term == "xterm-256color" || term == "screen-256color" {
		return termenv.ANSI256
	}

	// Use automatic detection as fallback
	// This will check TERM, COLORTERM, and other terminal capabilities
	profile := termenv.EnvColorProfile()

	return profile
}

// configureColors sets up the color profile for lipgloss based on environment detection.
// ConfigureColors sets up the color profile for terminal output.
func ConfigureColors() termenv.Profile {
	profile := detectColorProfile()
	lipgloss.SetColorProfile(profile)

	// Return profile so caller can configure logger if needed
	return profile
}

// ProfileName returns a human-readable name for the color profile.
func ProfileName(profile termenv.Profile) string {
	switch profile {
	case termenv.Ascii:
		return "ASCII (no color)"
	case termenv.ANSI:
		return "ANSI (16 colors)"
	case termenv.ANSI256:
		return "ANSI256 (256 colors)"
	case termenv.TrueColor:
		return "TrueColor (16M colors)"
	default:
		return "Unknown"
	}
}

// IsGitHubActions detects if we're running in GitHub Actions.
func IsGitHubActions() bool {
	return viper.GetString("GITHUB_ACTIONS") != ""
}

// IsCI detects if we're running in any CI environment.
func IsCI() bool {
	// Check common CI environment variables
	ciVars := []string{
		"CI",
		"CONTINUOUS_INTEGRATION",
		"BUILD_NUMBER",
		"JENKINS_URL",
		"TRAVIS",
		"CIRCLECI",
		"GITLAB_CI",
		"APPVEYOR",
		"BUILDKITE",
		"DRONE",
	}

	for _, envVar := range ciVars {
		if value := viper.GetString(envVar); value != "" && value != "false" {
			return true
		}
	}

	return false
}
