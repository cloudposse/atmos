package ui

import "strings"

// InlineCode wraps text in backticks so that markdown-rendering UI output shows it as code.
// A backtick inside the text would end the span early, so it is replaced with an apostrophe.
func InlineCode(text string) string {
	return "`" + strings.ReplaceAll(text, "`", "'") + "`"
}
