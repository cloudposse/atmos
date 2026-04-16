package env

import (
	"fmt"

	"al.essio.dev/pkg/shellescape"

	ghactions "github.com/cloudposse/atmos/pkg/github/actions"
)

// formatEnvValue formats a key-value pair as key=value (no quoting).
func formatEnvValue(key, value string) string {
	return fmt.Sprintf("%s=%s\n", key, value)
}

// formatDotenvValue formats a key-value pair as key=value with shell-safe quoting.
// Uses shellescape.Quote which adds quotes only when needed and handles all escaping.
func formatDotenvValue(key, value string) string {
	return fmt.Sprintf("%s=%s\n", key, shellescape.Quote(value))
}

// formatBashValue formats a key-value pair as export key=value with shell-safe quoting.
// Uses shellescape.Quote which adds quotes only when needed and handles all escaping.
// If cfg.exportPrefix is explicitly set to false, omits the 'export' prefix.
func formatBashValue(key, value string, cfg *config) string {
	quoted := shellescape.Quote(value)
	// Default to export=true if not explicitly set.
	if cfg != nil && cfg.exportPrefix != nil && !*cfg.exportPrefix {
		return fmt.Sprintf("%s=%s\n", key, quoted)
	}
	return fmt.Sprintf("export %s=%s\n", key, quoted)
}

// formatGitHubValue formats a key-value pair for GitHub Actions.
// Delegates to pkg/github/actions for GitHub-specific formatting.
func formatGitHubValue(key, value string) string {
	return ghactions.FormatValue(key, value)
}
