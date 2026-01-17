package actions

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// FormatValue formats a single key-value pair for GitHub Actions.
// Uses key=value for single-line values and heredoc syntax for multiline values.
// The heredoc delimiter is ATMOS_EOF_<key> with collision detection to ensure
// the delimiter doesn't appear in the value content.
func FormatValue(key, value string) string {
	defer perf.Track(nil, "github.actions.FormatValue")()

	if strings.Contains(value, "\n") {
		delimiter := fmt.Sprintf("ATMOS_EOF_%s", key)
		// Ensure delimiter doesn't collide with value content.
		for i := 0; strings.Contains(value, delimiter); i++ {
			delimiter = fmt.Sprintf("ATMOS_EOF_%s_%d", key, i)
		}
		return fmt.Sprintf("%s<<%s\n%s\n%s\n", key, delimiter, value, delimiter)
	}
	return fmt.Sprintf("%s=%s\n", key, value)
}

// FormatData formats a map of key-value pairs for GitHub Actions.
// Keys are sorted alphabetically for consistent output.
// Empty values are skipped.
func FormatData(data map[string]string) string {
	defer perf.Track(nil, "github.actions.FormatData")()

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, key := range keys {
		value := data[key]
		if value == "" {
			continue
		}
		sb.WriteString(FormatValue(key, value))
	}
	return sb.String()
}
