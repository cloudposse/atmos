package markdown

import (
	"strings"
)

// ParseMarkdownSections parses a markdown string and returns a map of section titles to their content
func ParseMarkdownSections(content string) map[string]string {
	sections := make(map[string]string)
	lines := strings.Split(content, "\n")

	var currentTitle string
	var currentContent []string

	for _, line := range lines {
		if strings.HasPrefix(line, "# ") {
			// If we have a previous section, save it
			if currentTitle != "" {
				sections[currentTitle] = strings.TrimSpace(strings.Join(currentContent, "\n"))
			}
			// Start new section
			currentTitle = strings.TrimPrefix(line, "# ")
			currentContent = []string{}
		} else if currentTitle != "" {
			currentContent = append(currentContent, line)
		}
	}

	// Save the last section
	if currentTitle != "" {
		sections[currentTitle] = strings.TrimSpace(strings.Join(currentContent, "\n"))
	}

	return sections
}

// SplitMarkdownContent splits markdown content into details and suggestion parts
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
