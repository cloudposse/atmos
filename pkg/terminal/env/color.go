package env

import "os"

// IsColorEnabled checks environment variables to determine if color should be enabled.
//
// Returns:
//   - false if NO_COLOR is set (disables color)
//   - false if CLICOLOR=0 is set (disables color, unless forced)
//   - true if CLICOLOR_FORCE or FORCE_COLOR is set (forces color)
//   - nil if no color-related environment variables are set (caller should use TTY detection)
//
// This function is designed for early initialization before flags or config are available.
// It only checks environment variables and has zero dependencies on other atmos packages.
func IsColorEnabled() *bool {
	//nolint:forbidigo // os.Getenv is acceptable for environment variable detection in this utility package
	noColor := os.Getenv("NO_COLOR")
	//nolint:forbidigo // os.Getenv is acceptable for environment variable detection in this utility package
	cliColor := os.Getenv("CLICOLOR")
	//nolint:forbidigo // os.Getenv is acceptable for environment variable detection in this utility package
	cliColorForce := os.Getenv("CLICOLOR_FORCE")
	//nolint:forbidigo // os.Getenv is acceptable for environment variable detection in this utility package
	forceColor := os.Getenv("FORCE_COLOR")

	// 1. NO_COLOR always wins - disables all color
	if noColor != "" {
		disabled := false
		return &disabled
	}

	// 2. CLICOLOR_FORCE or FORCE_COLOR - forces color even for non-TTY
	if cliColorForce != "" || forceColor != "" {
		enabled := true
		return &enabled
	}

	// 3. CLICOLOR=0 - disables color
	if cliColor == "0" {
		disabled := false
		return &disabled
	}

	// No environment variables set - caller should use TTY detection or other defaults
	return nil
}
