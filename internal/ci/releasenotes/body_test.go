package releasenotes

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const bodyWithCodeRabbit = "## what\n\nAdds a widget.\n\n<!-- a stray comment -->\n" +
	"<!-- This is an auto-generated comment: release notes by coderabbit.ai -->\n" +
	"## Summary by CodeRabbit\n\n* **New Features**\n  * A widget.\n" +
	"<!-- end of auto-generated comment: release notes by coderabbit.ai -->\n"

func TestCleanBody(t *testing.T) {
	got := CleanBody(bodyWithCodeRabbit)
	assert.Equal(t, "## what\n\nAdds a widget.", got)
	assert.NotContains(t, got, "coderabbit")
	assert.NotContains(t, got, "stray")
}

func TestCodeRabbitSummary(t *testing.T) {
	assert.Equal(t, "* **New Features**\n  * A widget.", CodeRabbitSummary(bodyWithCodeRabbit))
	assert.Equal(t, "", CodeRabbitSummary("no block here"))
}

func TestFallbackSummary(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "prefers CodeRabbit summary", body: bodyWithCodeRabbit, want: "* **New Features**\n  * A widget."},
		{name: "cleaned description otherwise", body: "Fixes it.\n<!-- hidden -->", want: "Fixes it."},
		{name: "empty body", body: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FallbackSummary(tt.body))
		})
	}
}

func TestFallbackSummary_TruncatesLongDescription(t *testing.T) {
	got := FallbackSummary(strings.Repeat("y", fallbackBodyChars*2))
	assert.Len(t, []rune(got), fallbackBodyChars+1)
	assert.True(t, strings.HasSuffix(got, "…"))
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "short", Truncate("short", 10))
	assert.Equal(t, "ab…", Truncate("abcdef", 2))
	// Counts runes, not bytes, so multibyte text is never cut mid-character.
	assert.Equal(t, "日本…", Truncate("日本語テキスト", 2))
	assert.Equal(t, "ends clean…", Truncate("ends clean   with spaces", 13))
}
