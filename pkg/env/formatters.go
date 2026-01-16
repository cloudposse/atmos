package env

import (
	"fmt"
	"strings"
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
func formatBashValue(key, value string) string {
	safe := EscapeSingleQuotes(value)
	return fmt.Sprintf("export %s='%s'\n", key, safe)
}

// formatGitHubValue formats a key-value pair for GitHub Actions.
// Uses key=value for single-line values and heredoc syntax for multiline values.
// The heredoc delimiter is ATMOS_EOF_<key> to avoid collision with values containing "EOF".
func formatGitHubValue(key, value string) string {
	if strings.Contains(value, "\n") {
		delimiter := fmt.Sprintf("ATMOS_EOF_%s", key)
		return fmt.Sprintf("%s<<%s\n%s\n%s\n", key, delimiter, value, delimiter)
	}
	return fmt.Sprintf("%s=%s\n", key, value)
}
