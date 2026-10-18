package markdown

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

// StripFrontmatter omits a leading YAML metadata block from rendered prose.
// Malformed or unclosed blocks and Markdown horizontal rules remain untouched.
// This transforms only the display text, never the source document on disk.
func StripFrontmatter(content string) string {
	defer perf.Track(nil, "markdown.StripFrontmatter")()

	text := strings.TrimPrefix(content, "\ufeff")
	first, remaining, found := strings.Cut(text, "\n")
	if !found || strings.TrimRight(first, "\r \t") != "---" {
		return content
	}
	start := len(first) + 1
	offset := start
	for remaining != "" {
		line, rest, more := strings.Cut(remaining, "\n")
		delimiter := strings.TrimRight(line, "\r \t")
		if delimiter == "---" || delimiter == "..." {
			var metadata map[string]any
			if err := yaml.Unmarshal([]byte(text[start:offset]), &metadata); err != nil {
				return content
			}
			return rest
		}
		if !more {
			break
		}
		offset += len(line) + 1
		remaining = rest
	}
	return content
}
