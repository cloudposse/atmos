package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/edition"
)

func TestParseOptionalAnchor(t *testing.T) {
	anchor, err := parseOptionalAnchor("")
	require.NoError(t, err)
	assert.Nil(t, anchor, "empty string means unbounded")

	anchor, err = parseOptionalAnchor("2026-01")
	require.NoError(t, err)
	require.NotNil(t, anchor)
	assert.Equal(t, "2026-01", anchor.Raw)

	_, err = parseOptionalAnchor("not-a-date")
	require.ErrorIs(t, err, edition.ErrInvalidEdition)
}

func TestEditionsToDataNewestFirst(t *testing.T) {
	entries := []edition.Entry{
		{Date: "2025-01-01", Key: "a", Kind: edition.KindValue, Old: true, New: false, Description: "first", Ref: "r1"},
		{Date: "2026-01-01", Key: "b", Kind: edition.KindValue, Old: "x", New: "y", Description: "second", Ref: "r2"},
	}

	data := editionsToData(entries)
	require.Len(t, data, 2)
	// Newest first.
	assert.Equal(t, "2026-01-01", data[0]["date"])
	assert.Equal(t, "b", data[0]["key"])
	assert.Equal(t, "x", data[0]["old"])
	assert.Equal(t, "y", data[0]["new"])
	assert.Equal(t, "2025-01-01", data[1]["date"])
	assert.Equal(t, "a", data[1]["key"])
	assert.Equal(t, "true", data[1]["old"])
	assert.Equal(t, "false", data[1]["new"])
}

func TestExecuteListEditionsWithOptionsInvalidAnchors(t *testing.T) {
	err := executeListEditionsWithOptions(&EditionsOptions{From: "13-2026"})
	require.ErrorIs(t, err, edition.ErrInvalidEdition)

	err = executeListEditionsWithOptions(&EditionsOptions{To: "2026-99"})
	require.ErrorIs(t, err, edition.ErrInvalidEdition)
}

func TestBuildEditionsFooter(t *testing.T) {
	tests := []struct {
		name string
		opts EditionsOptions
		want string
	}{
		{name: "no range", opts: EditionsOptions{}, want: "journaled"},
		{name: "range", opts: EditionsOptions{From: "2025", To: "2026"}, want: "between editions 2025 and 2026"},
		{name: "from only", opts: EditionsOptions{From: "2025"}, want: "since edition 2025"},
		{name: "to only", opts: EditionsOptions{To: "2026"}, want: "up to edition 2026"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, buildEditionsFooter(2, &tt.opts), tt.want)
		})
	}
}
