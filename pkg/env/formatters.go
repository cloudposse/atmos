package env

import (
	"fmt"

	ghactions "github.com/cloudposse/atmos/pkg/github/actions"
)

// formatEnvValue formats a key-value pair as key=value (no quoting).
func formatEnvValue(key, value string) string {
	return fmt.Sprintf("%s=%s\n", key, value)
}

// formatDotenvValue formats a key-value pair as key='value' with single-quote escaping.
func formatDotenvValue(key, value string) string {
	safe := EscapeSingleQuotes(value)
	return fmt.Sprintf("%s='%s'\n", key, safe)
}

// formatBashValue formats a key-value pair as export key='value' with single-quote escaping.
// If cfg.exportPrefix is explicitly set to false, omits the 'export' prefix.
func formatBashValue(key, value string, cfg *config) string {
	safe := EscapeSingleQuotes(value)
	// Default to export=true if not explicitly set.
	if cfg != nil && cfg.exportPrefix != nil && !*cfg.exportPrefix {
		return fmt.Sprintf("%s='%s'\n", key, safe)
	}
	return fmt.Sprintf("export %s='%s'\n", key, safe)
}

// formatGitHubValue formats a key-value pair for GitHub Actions.
// Delegates to pkg/github/actions for GitHub-specific formatting.
func formatGitHubValue(key, value string) string {
	return ghactions.FormatValue(key, value)
}
