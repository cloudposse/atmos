package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
	"github.com/muesli/termenv"
	"github.com/spf13/viper"
)

// detectColorProfile determines the appropriate color profile based on environment.
func detectColorProfile() termenv.Profile {
	// Check if colors are explicitly disabled via --no-color flag or NO_COLOR env
	noColorFlag := viper.GetBool("no_color")
	if noColorFlag || config.NoColor() {
		return termenv.Ascii
	}

	// Check if colors are explicitly enabled
	if config.ForceColor() {
		// FORCE_COLOR can have values like "1", "2", "3" for different color levels
		switch viper.GetString("force.color") {
		case "2":
			return termenv.ANSI256
		case "3":
			return termenv.TrueColor
		default:
			return termenv.ANSI
		}
	}

	// Special handling for GitHub Actions and other CI environments
	if config.IsGitHubActions() {
		// GitHub Actions supports ANSI colors
		// Check if TERM suggests 256 color support
		if term := viper.GetString("term"); term == "xterm-256color" || term == "screen-256color" {
			return termenv.ANSI256
		}
		return termenv.ANSI
	}

	// Generic CI detection - most CI environments support at least ANSI colors
	if config.IsCI() {
		return termenv.ANSI
	}

	// For local development, check for explicit color support indicators
	// Check for truecolor support
	if colorterm := viper.GetString("colorterm"); colorterm == "truecolor" || colorterm == "24bit" {
		return termenv.TrueColor
	}

	// Check TERM for 256 color support
	if term := viper.GetString("term"); term == "xterm-256color" || term == "screen-256color" {
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
	// Use the config package for CI detection
	return config.IsGitHubActions()
}

// IsCI detects if we're running in any CI environment.
func IsCI() bool {
	// Use the config package for CI detection
	return config.IsCI()
}
