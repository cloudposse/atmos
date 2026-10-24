package plugin

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncateDetail(t *testing.T) {
	assert.Empty(t, TruncateDetail(""))

	short := "small body"
	assert.Equal(t, short, TruncateDetail(short))

	longValue := strings.Repeat("x", DetailOutputMaxBytes+4)
	assert.True(t, strings.HasPrefix(TruncateDetail(longValue), "... output truncated ..."))

	// Truncation aligns the cut to a line boundary, preserving whole tail lines.
	line := `resource "google_cloud_run_v2_job" "run_job" {`
	lineAlignedValue := strings.Repeat(line+"\n", DetailOutputMaxBytes/len(line)+10)
	lineAlignedTail := strings.TrimPrefix(TruncateDetail(lineAlignedValue), DetailTruncatedPrefix)
	firstLine, _, _ := strings.Cut(lineAlignedTail, "\n")
	assert.Equal(t, line, firstLine)
}
