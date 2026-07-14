package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatColumns_Empty(t *testing.T) {
	assert.Equal(t, "", FormatColumns(nil, 80))
	assert.Equal(t, "", FormatColumns([]string{}, 80))
}

func TestFormatColumns_FallsBackToDefaultWidth(t *testing.T) {
	items := []string{"aa", "bb", "cc"}

	withZero := FormatColumns(items, 0)
	withDefault := FormatColumns(items, columnDefaultWidth)
	assert.Equal(t, withDefault, withZero)

	withNegative := FormatColumns(items, -10)
	assert.Equal(t, withDefault, withNegative)
}

func TestFormatColumns_SingleColumnWhenNarrow(t *testing.T) {
	items := []string{"atmos-terraform", "atmos-helmfile", "atmos-packer"}

	got := FormatColumns(items, 10) // Narrower than any single item.
	lines := strings.Split(got, "\n")
	assert.Len(t, lines, len(items), "one item per line when nothing fits side by side")
	for i, line := range lines {
		assert.Equal(t, items[i], line, "no trailing padding on a single-column layout")
	}
}

func TestFormatColumns_GridLayoutIsColumnMajor(t *testing.T) {
	// 6 items, forced to exactly 2 columns via width: colWidth = len("ccc")+2 = 5,
	// so width=10 allows exactly 2 columns (2*5=10, 3*5=15>10).
	items := []string{"a", "bb", "ccc", "d", "ee", "fff"}

	got := FormatColumns(items, 10)
	lines := strings.Split(got, "\n")
	assert.Len(t, lines, 3, "6 items in 2 columns needs 3 rows")

	// Column-major (ls-style): column 0 gets the first 3 items top-to-bottom,
	// column 1 gets the next 3 -- not left-to-right row order.
	assert.True(t, strings.HasPrefix(lines[0], "a "))
	assert.Contains(t, lines[0], "d")
	assert.True(t, strings.HasPrefix(lines[1], "bb"))
	assert.Contains(t, lines[1], "ee")
	assert.True(t, strings.HasPrefix(lines[2], "ccc"))
	assert.True(t, strings.HasSuffix(lines[2], "fff"), "last column entry has no trailing padding")
}

func TestFormatColumns_NoTrailingWhitespacePerLine(t *testing.T) {
	items := []string{"atmos-ai", "atmos-terraform", "atmos-git", "atmos-packer", "atmos-cache"}

	got := FormatColumns(items, 40)
	for _, line := range strings.Split(got, "\n") {
		assert.Equal(t, strings.TrimRight(line, " "), line, "line must not end in padding: %q", line)
	}
}

func TestFormatColumns_EveryItemAppearsExactlyOnce(t *testing.T) {
	items := []string{
		"atmos-ai", "atmos-ansible", "atmos-asciicast", "atmos-auth", "atmos-aws-compliance",
		"atmos-aws-ecr", "atmos-aws-eks", "atmos-aws-security", "atmos-cache", "atmos-cast",
	}

	got := FormatColumns(items, 90)
	for _, item := range items {
		assert.Equal(t, 1, strings.Count(got, item), "item %q should appear exactly once", item)
	}
}
