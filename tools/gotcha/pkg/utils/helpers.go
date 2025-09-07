package utils

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"

	"github.com/mattn/go-isatty"
	"github.com/spf13/viper"
)

// IsValidShowFilter validates that the show filter is one of the allowed values.
func IsValidShowFilter(show string) bool {
	validFilters := []string{"all", "failed", "passed", "skipped", "collapsed", "none"}
	for _, valid := range validFilters {
		if show == valid {
			return true
		}
	}
	return false
}

// FilterPackages applies include/exclude regex patterns to filter packages.
func FilterPackages(packages []string, includePatterns, excludePatterns string) ([]string, error) {
	// If no packages provided, return as-is
	if len(packages) == 0 {
		return packages, nil
	}

	// Parse include patterns
	var includeRegexes []*regexp.Regexp
	if includePatterns != "" {
		for _, pattern := range strings.Split(includePatterns, ",") {
			pattern = strings.TrimSpace(pattern)
			if pattern != "" {
				regex, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("%w: '%s': %v", types.ErrInvalidIncludePattern, pattern, err)
				}
				includeRegexes = append(includeRegexes, regex)
			}
		}
	}

	// Parse exclude patterns
	var excludeRegexes []*regexp.Regexp
	if excludePatterns != "" {
		for _, pattern := range strings.Split(excludePatterns, ",") {
			pattern = strings.TrimSpace(pattern)
			if pattern != "" {
				regex, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("%w: '%s': %v", types.ErrInvalidExcludePattern, pattern, err)
				}
				excludeRegexes = append(excludeRegexes, regex)
			}
		}
	}

	// If no patterns specified, return original packages
	if len(includeRegexes) == 0 && len(excludeRegexes) == 0 {
		return packages, nil
	}

	// Filter packages
	var filtered []string
	for _, pkg := range packages {
		// Check include patterns (if any)
		included := len(includeRegexes) == 0 // Default to include if no include patterns
		for _, regex := range includeRegexes {
			if regex.MatchString(pkg) {
				included = true
				break
			}
		}

		// Check exclude patterns (if any)
		excluded := false
		for _, regex := range excludeRegexes {
			if regex.MatchString(pkg) {
				excluded = true
				break
			}
		}

		// Include if it matches include patterns and doesn't match exclude patterns
		if included && !excluded {
			filtered = append(filtered, pkg)
		}
	}

	return filtered, nil
}

// IsTTY checks if we're running in a terminal and Bubble Tea can actually use it.
// Uses cross-platform detection that works on Windows, macOS, and Linux.
func IsTTY() bool {
	// Provide an environment override
	_ = viper.BindEnv("GOTCHA_FORCE_NO_TTY", "FORCE_NO_TTY")
	if viper.GetString("GOTCHA_FORCE_NO_TTY") != "" {
		return false
	}

	// Debug: Force TTY mode for testing (but only if TTY is actually usable)
	_ = viper.BindEnv("GOTCHA_FORCE_TTY", "FORCE_TTY")
	if viper.GetString("GOTCHA_FORCE_TTY") != "" {
		// Use cross-platform TTY detection
		stdoutTTY := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
		stdinTTY := isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
		return stdoutTTY && stdinTTY
	}

	// Cross-platform TTY detection using go-isatty
	// Works correctly on Windows, macOS, and Linux
	stdoutTTY := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
	stdinTTY := isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())

	// Windows CI environments often report as TTY but aren't really interactive
	// Disable TUI mode in CI environments on Windows to prevent issues
	if runtime.GOOS == "windows" && (os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "") {
		return false
	}

	return stdoutTTY && stdinTTY
}

// EmitAlert outputs a terminal bell (\a) if alert is enabled.
func EmitAlert(enabled bool) {
	if enabled {
		fmt.Fprint(os.Stderr, "\a")
	}
}

// Note: The streaming functionality has been moved to the stream package
// to better organize the code and meet file length requirements.