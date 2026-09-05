package releasenotes

import (
	"regexp"
	"strings"
)

const (
	// Bounds each entry's rendered body when no model is available: enough
	// for the gist, small enough that even a hundred entries stay far below
	// GitHub's release-body limit.
	fallbackBodyChars = 1200

	// The ceiling this package renders to. GitHub rejects release bodies
	// over 125,000 characters; the margin covers anything the API counts
	// differently than len() does.
	maxReleaseBodyChars = 120000
)

var (
	// CodeRabbit appends its own summary to PR descriptions between these
	// markers. It is stripped from what the model sees (the model should
	// summarize the author's description, not another summary) and used as
	// the ready-made fallback when there is no model.
	coderabbitBlock = regexp.MustCompile(`(?s)<!--\s*This is an auto-generated comment: release notes by coderabbit\.ai\s*-->(.*?)<!--\s*end of auto-generated comment: release notes by coderabbit\.ai\s*-->`)
	htmlComment     = regexp.MustCompile(`(?s)<!--.*?-->`)
	coderabbitTitle = regexp.MustCompile(`(?m)^##\s*Summary by CodeRabbit\s*$`)
)

// CleanBody returns a PR description with CodeRabbit's auto-generated block
// and any other HTML comments removed, trimmed.
func CleanBody(body string) string {
	body = coderabbitBlock.ReplaceAllString(body, "")
	body = htmlComment.ReplaceAllString(body, "")
	return strings.TrimSpace(body)
}

// CodeRabbitSummary returns the text of CodeRabbit's "Summary by CodeRabbit"
// block from a PR description, without its heading, or "" when there is
// none.
func CodeRabbitSummary(body string) string {
	m := coderabbitBlock.FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(coderabbitTitle.ReplaceAllString(m[1], ""))
}

// FallbackSummary is what an entry gets instead of a model summary:
// CodeRabbit's summary when the PR has one, else the description itself,
// either way bounded to fallbackBodyChars.
func FallbackSummary(body string) string {
	if s := CodeRabbitSummary(body); s != "" {
		return Truncate(s, fallbackBodyChars)
	}
	return Truncate(CleanBody(body), fallbackBodyChars)
}

// Truncate cuts s to at most n runes, marking the cut with an ellipsis.
func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}
