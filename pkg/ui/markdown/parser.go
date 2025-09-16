package markdown

import (
	"strings"
)

// SplitMarkdownContent splits markdown content into details and suggestion parts.
func SplitMarkdownContent(content string) []string {
	parts := strings.Split(content, "\n\n")
	var result []string

	// First non-empty line is details
	for i, part := range parts {
		if strings.TrimSpace(part) != "" {
			result = append(result, strings.TrimSpace(part))
			if i < len(parts)-1 {
				// Rest is suggestion
				result = append(result, strings.TrimSpace(strings.Join(parts[i+1:], "\n\n")))
			}
			break
		}
	}

	return result
}
