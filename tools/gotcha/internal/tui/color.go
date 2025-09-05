package tui

import (
	"os"

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

	// Check if colors are explicitly disabled via --no-color flag or NO_COLOR env
	noColorFlag := viper.GetBool("no_color")
	noColorEnv := viper.GetString("NO_COLOR")
	// NO_COLOR should only disable colors if it's set to a truthy value
	// The standard says any non-empty value disables colors, but some tools set it to "false" to enable colors
	// We'll treat "false", "0", and empty string as NOT disabling colors
	noColorSet := noColorEnv != "" && noColorEnv != "false" && noColorEnv != "0"
	if noColorFlag || noColorSet {
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

	// Default to ANSI colors for modern terminals
	// Even when piping, most modern terminals can handle ANSI codes
	// Users can explicitly disable with --no-color if needed
	return termenv.ANSI
}

// configureColors sets up the color profile for lipgloss based on environment detection.
// ConfigureColors sets up the color profile for terminal output.
func ConfigureColors() termenv.Profile {
	profile := detectColorProfile()

	// Set the color profile for lipgloss
	// This works even when output is piped, unlike custom renderers
	lipgloss.SetColorProfile(profile)

	// Clear any custom renderer to use the global profile
	SetRenderer(nil)

	// Reinitialize styles with the new color profile
	InitStyles()

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
	// Use os.Getenv directly for CI detection to ensure it works before viper is fully configured
	return os.Getenv("GITHUB_ACTIONS") != ""
}

// IsCI detects if we're running in any CI environment.
func IsCI() bool {
	// Check common CI environment variables using os.Getenv directly
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
		if value := os.Getenv(envVar); value != "" && value != "false" {
			return true
		}
	}

	return false
}
