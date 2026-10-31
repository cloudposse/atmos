package report

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/dustin/go-humanize"
	"github.com/opencontainers/go-digest"

	atmosio "github.com/cloudposse/atmos/pkg/io"
)

const (
	// The shortDigestLength constant is the number of hex characters of a digest shown in a table cell.
	shortDigestLength = 12

	// The minFenceLength constant is the shortest Markdown code fence.
	minFenceLength = 3

	// The backtick constant is the Markdown code delimiter character.
	backtick = "`"

	// The ellipsis constant marks a shortened value.
	ellipsis = "…"

	// The millisPerSecond constant converts milliseconds to seconds for duration formatting.
	millisPerSecond = 1000
)

// urlPattern finds URLs in free text so credentials and query strings can be removed from them.
// The scheme is required, so a bare OCI reference such as ghcr.io/org/repo:1.0 is never touched.
var urlPattern = regexp.MustCompile("[a-zA-Z][a-zA-Z0-9+.-]*://[^\\s\"'<>`]+")

// markdownEscaper backslash-escapes the characters that change meaning in running Markdown text and
// in table cells, and turns the HTML-significant characters into entities.
var markdownEscaper = strings.NewReplacer(
	`\`, `\\`,
	"`", "\\`",
	"*", `\*`,
	"_", `\_`,
	"[", `\[`,
	"]", `\]`,
	"(", `\(`,
	")", `\)`,
	"|", `\|`,
	"~", `\~`,
	"#", `\#`,
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	"://", "&#58;//",
)

// scrub masks secrets, removes credentials and query strings from URLs, and collapses every
// control character (including newlines) to a single space. It is the first step of every value
// that reaches the report, so a signed URL or a secret never reaches the template.
func scrub(value string) string {
	value = atmosio.MaskString(value)
	value = redactURLs(value)
	return collapse(value)
}

// collapse replaces runs of whitespace and control characters with one space and trims the result.
func collapse(value string) string {
	var builder strings.Builder
	space := false
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			space = builder.Len() > 0
			continue
		}
		if space {
			builder.WriteByte(' ')
			space = false
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

// redactURLs drops the user information, query string, and fragment of every URL in value. Those
// parts carry presigned-URL signatures and tokens, and a summary must never contain them.
func redactURLs(value string) string {
	return urlPattern.ReplaceAllStringFunc(value, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			// An unparsable URL cannot be cleaned reliably, so cut it at the first query or fragment mark.
			if index := strings.IndexAny(raw, "?#"); index >= 0 {
				return raw[:index]
			}
			return raw
		}
		if parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" && !parsed.ForceQuery {
			return raw
		}
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		return parsed.String()
	})
}

// plain renders untrusted text as escaped Markdown that is safe in running text and in table cells.
func plain(value string) string {
	return markdownEscaper.Replace(scrub(value))
}

// code renders untrusted text as an inline code span. The span uses a backtick run longer than any
// run inside the value, so the value can never close the span or open an HTML tag.
func code(value string) string {
	value = scrub(value)
	if value == "" {
		return ""
	}
	delimiter := strings.Repeat(backtick, longestBacktickRun(value)+1)
	if strings.HasPrefix(value, backtick) || strings.HasSuffix(value, backtick) {
		value = " " + value + " "
	}
	return delimiter + value + delimiter
}

// longestBacktickRun returns the length of the longest run of backticks in value.
func longestBacktickRun(value string) int {
	longest, current := 0, 0
	for _, r := range value {
		if r == '`' {
			current++
			longest = max(longest, current)
			continue
		}
		current = 0
	}
	return longest
}

// fenced renders untrusted multi-line text as a fenced code block that the text cannot close. The
// text keeps its line breaks but is masked and stripped of URL credentials and query strings.
func fenced(value string) string {
	value = atmosio.MaskString(value)
	value = redactURLs(value)
	value = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, value))
	if value == "" {
		return ""
	}
	fence := strings.Repeat(backtick, max(minFenceLength, longestBacktickRun(value)+1))
	return fence + "text\n" + value + "\n" + fence
}

// shortDigest shortens a digest for a table cell and keeps its algorithm, for example sha256:0123456789ab….
func shortDigest(value digest.Digest) string {
	text := string(value)
	algorithm, hex, found := strings.Cut(text, ":")
	if !found || len(hex) <= shortDigestLength {
		return text
	}
	return algorithm + ":" + hex[:shortDigestLength] + ellipsis
}

// humanSize formats a byte count for people; zero is reported as empty because an operation that
// transferred nothing has no size to show.
func humanSize(size int64) string {
	if size <= 0 {
		return ""
	}
	return humanize.IBytes(uint64(size))
}

// exactSize formats a byte count with its exact value, for the details view.
func exactSize(size int64) string {
	if size <= 0 {
		return ""
	}
	return fmt.Sprintf("%s (%s bytes)", humanize.IBytes(uint64(size)), humanize.Comma(size))
}

// formatDuration formats an operation duration compactly; zero is reported as empty.
func formatDuration(duration time.Duration) string {
	switch {
	case duration <= 0:
		return ""
	case duration < time.Millisecond:
		return "<1 ms"
	case duration < time.Second:
		return fmt.Sprintf("%d ms", duration.Milliseconds())
	case duration < time.Minute:
		return fmt.Sprintf("%.1f s", float64(duration.Milliseconds())/millisPerSecond)
	default:
		return duration.Round(time.Second).String()
	}
}
