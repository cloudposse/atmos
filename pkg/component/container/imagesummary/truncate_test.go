package imagesummary

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTruncateCommentBoundary(t *testing.T) {
	t.Run("a body at the limit is unchanged", func(t *testing.T) {
		body := strings.Repeat("a", MaxCommentChars)
		assert.Equal(t, body, TruncateComment(body))
	})

	t.Run("a body one character over the limit is truncated within the limit", func(t *testing.T) {
		body := strings.Repeat("a", MaxCommentChars+1)
		got := TruncateComment(body)
		assert.NotEqual(t, body, got)
		assert.LessOrEqual(t, len([]rune(got)), MaxCommentChars)
		assert.True(t, strings.HasSuffix(got, "_Comment truncated; see the job summary for the full report._\n"))
		assert.True(t, strings.HasPrefix(got, "aaaa"))
	})

	t.Run("a short body is unchanged", func(t *testing.T) {
		assert.Equal(t, "short", TruncateComment("short"))
		assert.Empty(t, TruncateComment(""))
	})
}

func TestTruncateCommentKeepsMultibyteCharactersIntact(t *testing.T) {
	body := strings.Repeat("🐳", MaxCommentChars+10)
	got := TruncateComment(body)

	assert.LessOrEqual(t, len([]rune(got)), MaxCommentChars)
	require.True(t, strings.HasSuffix(got, "full report._\n"))
	trimmed := strings.TrimSuffix(got, truncationNote)
	assert.Equal(t, strings.Repeat("🐳", len([]rune(trimmed))), trimmed, "the cut never splits a rune")
}

func TestTruncateCommentClosesOpenFenceAndSections(t *testing.T) {
	body := "<details>\n<summary>Raw JSON</summary>\n\n```json\n" + strings.Repeat("x", MaxCommentChars)
	got := TruncateComment(body)

	assert.LessOrEqual(t, len([]rune(got)), MaxCommentChars)
	assert.Equal(t, 0, strings.Count(got, "```")%2, "the code fence is balanced")
	assert.Equal(t, strings.Count(got, "<details>"), strings.Count(got, "</details>"), "the collapsible section is closed")
	assert.True(t, strings.HasSuffix(got, truncationNote))
	closeIdx := strings.LastIndex(got, "```")
	assert.Less(t, closeIdx, strings.Index(got, "_Comment truncated"), "the note follows the closing fence")
}

func TestTruncateCommentLeavesBalancedFencesAlone(t *testing.T) {
	body := "```\nblock\n```\n" + strings.Repeat("y", MaxCommentChars)
	got := TruncateComment(body)

	assert.Equal(t, 2, strings.Count(got, "```"))
}
