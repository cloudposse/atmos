package list

import (
	"testing"

	"github.com/charmbracelet/bubbles/table"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// withTableSeams overrides the TTY and terminal-width seams for the duration
// of a test and restores them on cleanup.
func withTableSeams(t *testing.T, tty bool, width int) {
	t.Helper()

	origTTY := isTTYForTable
	origWidth := terminalWidthForTable
	isTTYForTable = func() bool { return tty }
	terminalWidthForTable = func() int { return width }
	t.Cleanup(func() {
		isTTYForTable = origTTY
		terminalWidthForTable = origWidth
	})
}

func TestComputeColumnWidths(t *testing.T) {
	specs := []columnSizingSpec{
		{title: "NAME", legacy: 15, floor: 10},
		{title: "START URL / URL", legacy: 35, floor: 15},
		{title: "DEFAULT", legacy: 7, floor: 7},
	}
	rows := []table.Row{
		{"aws-sso", "https://d-abc123.awsapps.com/start/very/long/path/segment", "✓"},
		{"okta", "https://company.okta.com/app", ""},
	}
	longURLWidth := len("https://d-abc123.awsapps.com/start/very/long/path/segment") // 58.
	shrinkOrder := []int{1, 0}

	tests := []struct {
		name     string
		tty      bool
		width    int
		expected []int
	}{
		{
			name:  "non-TTY uses legacy fixed widths",
			tty:   false,
			width: 200,
			// Legacy widths returned verbatim, regardless of content or terminal width.
			expected: []int{15, 35, 7},
		},
		{
			name:  "wide terminal sizes columns to content",
			tty:   true,
			width: 200,
			// NAME grows to its widest cell, URL to the full URL, DEFAULT to its header.
			expected: []int{7, longURLWidth, 7},
		},
		{
			name:  "narrow terminal shrinks URL column first",
			tty:   true,
			width: 60,
			// Needed: 7+58+7 + 3*2 padding = 78; excess 18 comes entirely out of URL (58-18=40).
			expected: []int{7, 40, 7},
		},
		{
			name:  "very narrow terminal stops at floors",
			tty:   true,
			width: 20,
			// URL bottoms out at its floor (15); NAME is already below its floor
			// (content width 7 < floor 10) so it is never reduced; DEFAULT never shrinks.
			expected: []int{7, 15, 7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withTableSeams(t, tt.tty, tt.width)

			widths := computeColumnWidths(specs, rows, shrinkOrder)

			assert.Equal(t, tt.expected, widths)
		})
	}
}

func TestFitColumnWidths_ShrinkOrderSpillsToNextColumn(t *testing.T) {
	specs := []columnSizingSpec{
		{title: "A", legacy: 10, floor: 5},
		{title: "B", legacy: 10, floor: 5},
	}
	rows := []table.Row{
		{"aaaaaaaaaa", "bbbbbbbbbb"}, // Both columns need 10.
	}

	// Needed: 10+10 + 2*2 padding = 24. Terminal 12 → excess 12.
	// Column 0 gives up 5 (10→floor 5), the remaining 7 comes from column 1,
	// which also bottoms out at its floor.
	widths := fitColumnWidths(specs, rows, []int{0, 1}, 12)

	assert.Equal(t, []int{5, 5}, widths)
}

func TestFitColumnWidths_HeaderSetsMinimumContentWidth(t *testing.T) {
	specs := []columnSizingSpec{
		{title: "VIA PROVIDER", legacy: 18, floor: 12},
	}
	rows := []table.Row{
		{"x"}, // Content narrower than the header.
	}

	widths := fitColumnWidths(specs, rows, []int{0}, 200)

	// The column is never narrower than its header.
	assert.Equal(t, []int{len("VIA PROVIDER")}, widths)
}

func TestCreateProvidersTable_WideTerminalNoTruncation(t *testing.T) {
	withTableSeams(t, true, 200)

	longURL := "https://d-abc123.awsapps.com/start/with/a/very/long/path/for/testing"
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:     "aws/iam-identity-center",
			Region:   "us-east-1",
			StartURL: longURL,
			Default:  true,
		},
	}

	tbl, err := createProvidersTable(providers)
	require.NoError(t, err)

	view := tbl.View()
	assert.Contains(t, view, longURL, "wide terminal must render the full URL without truncation")
	assert.NotContains(t, view, "...", "wide terminal must not pre-truncate URLs")
}

func TestCreateProvidersTable_NarrowTerminalTruncatesURL(t *testing.T) {
	withTableSeams(t, true, 60)

	longURL := "https://d-abc123.awsapps.com/start/with/a/very/long/path/for/testing"
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:     "aws/iam-identity-center",
			Region:   "us-east-1",
			StartURL: longURL,
			Default:  true,
		},
	}

	tbl, err := createProvidersTable(providers)
	require.NoError(t, err)

	view := tbl.View()
	assert.NotContains(t, view, longURL, "narrow terminal must truncate the URL")
	// The URL column shrinks first but never below its floor, so the URL prefix survives.
	assert.Contains(t, view, longURL[:minProviderURLWidth-1], "URL column must keep at least its floor width")
	assert.Contains(t, view, "aws-sso", "provider name must not be truncated before the URL")
}

func TestCreateProvidersTable_NonTTYKeepsLegacyFixedWidths(t *testing.T) {
	withTableSeams(t, false, 200)

	longURL := "https://d-abc123.awsapps.com/start/with/a/very/long/path/for/testing"
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:     "aws/iam-identity-center",
			Region:   "us-east-1",
			StartURL: longURL,
			Default:  true,
		},
	}

	tbl, err := createProvidersTable(providers)
	require.NoError(t, err)

	view := tbl.View()
	// Legacy behavior: URLs are pre-truncated to maxURLDisplay with "..." even
	// on a wide (virtual) terminal, so piped output stays byte-stable.
	assert.NotContains(t, view, longURL)
	assert.Contains(t, view, truncateString(longURL, maxURLDisplay))
}

func TestCreateIdentitiesTable_WideTerminalNoTruncation(t *testing.T) {
	withTableSeams(t, true, 200)

	longName := "very-long-identity-name-that-exceeds-legacy-width"
	identities := map[string]schema.Identity{
		longName: {
			Kind:    "aws/permission-set",
			Default: true,
			Via:     &schema.IdentityVia{Provider: "aws-sso-with-a-long-provider-name"},
		},
	}

	tbl, err := createIdentitiesTable(nil, identities)
	require.NoError(t, err)

	view := tbl.View()
	assert.Contains(t, view, longName, "wide terminal must render the full identity name")
	assert.Contains(t, view, "aws-sso-with-a-long-provider-name", "wide terminal must render the full via-provider")
}

func TestCreateIdentitiesTable_NonTTYKeepsLegacyFixedWidths(t *testing.T) {
	withTableSeams(t, false, 200)

	longName := "very-long-identity-name-that-exceeds-legacy-width"
	identities := map[string]schema.Identity{
		longName: {
			Kind: "aws/permission-set",
			Via:  &schema.IdentityVia{Provider: "aws-sso"},
		},
	}

	tbl, err := createIdentitiesTable(nil, identities)
	require.NoError(t, err)

	view := tbl.View()
	// Legacy fixed width truncates the long name to the identityNameWidth column.
	assert.NotContains(t, view, longName)
	assert.Contains(t, view, longName[:identityNameWidth-1], "legacy column keeps the name prefix")
}

func TestBuildProviderRows_LegacyURLLimit(t *testing.T) {
	longURL := "https://d-abc123.awsapps.com/start/with/a/very/long/path"
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:     "aws/iam-identity-center",
			StartURL: longURL,
		},
	}

	rows := buildProviderRows(providers, maxURLDisplay)

	require.Len(t, rows, 1)
	assert.Equal(t, truncateString(longURL, maxURLDisplay), rows[0][3])
}
