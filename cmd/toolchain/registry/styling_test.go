package registry

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestToolRowImplementsRowStyler verifies that toolRow implements rowStyler.
func TestToolRowImplementsRowStyler(t *testing.T) {
	row := toolRow{
		isInstalled: true,
		isInConfig:  true,
	}

	// Verify interface compliance.
	var _ rowStyler = row

	assert.True(t, row.GetIsInstalled())
	assert.True(t, row.GetIsInConfig())
}

// TestSearchRowImplementsRowStyler verifies that searchRow implements rowStyler.
func TestSearchRowImplementsRowStyler(t *testing.T) {
	row := searchRow{
		isInstalled: false,
		isInConfig:  true,
	}

	// Verify interface compliance.
	var _ rowStyler = row

	assert.False(t, row.GetIsInstalled())
	assert.True(t, row.GetIsInConfig())
}

// TestToolRowGetIsInstalled tests the GetIsInstalled method of toolRow.
func TestToolRowGetIsInstalled(t *testing.T) {
	tests := []struct {
		name        string
		isInstalled bool
		want        bool
	}{
		{"installed", true, true},
		{"not installed", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := toolRow{isInstalled: tt.isInstalled}
			assert.Equal(t, tt.want, row.GetIsInstalled())
		})
	}
}

// TestToolRowGetIsInConfig tests the GetIsInConfig method of toolRow.
func TestToolRowGetIsInConfig(t *testing.T) {
	tests := []struct {
		name       string
		isInConfig bool
		want       bool
	}{
		{"in config", true, true},
		{"not in config", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := toolRow{isInConfig: tt.isInConfig}
			assert.Equal(t, tt.want, row.GetIsInConfig())
		})
	}
}

// TestSearchRowGetIsInstalled tests the GetIsInstalled method of searchRow.
func TestSearchRowGetIsInstalled(t *testing.T) {
	tests := []struct {
		name        string
		isInstalled bool
		want        bool
	}{
		{"installed", true, true},
		{"not installed", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := searchRow{isInstalled: tt.isInstalled}
			assert.Equal(t, tt.want, row.GetIsInstalled())
		})
	}
}

// TestSearchRowGetIsInConfig tests the GetIsInConfig method of searchRow.
func TestSearchRowGetIsInConfig(t *testing.T) {
	tests := []struct {
		name       string
		isInConfig bool
		want       bool
	}{
		{"in config", true, true},
		{"not in config", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := searchRow{isInConfig: tt.isInConfig}
			assert.Equal(t, tt.want, row.GetIsInConfig())
		})
	}
}

// TestRenderTableWithConditionalStyling_ToolRows tests styling with toolRow slice.
func TestRenderTableWithConditionalStyling_ToolRows(t *testing.T) {
	tests := []struct {
		name        string
		tableView   string
		rows        []toolRow
		checkOutput func(t *testing.T, output string)
	}{
		{
			name: "applies styling to installed tool",
			tableView: `HEADER LINE
-----------
● hashicorp  terraform   github_release`,
			rows: []toolRow{
				{status: "●", isInstalled: true, isInConfig: true},
			},
			checkOutput: func(t *testing.T, output string) {
				// Should still contain the header and data.
				assert.Contains(t, output, "HEADER LINE")
				assert.Contains(t, output, "hashicorp")
			},
		},
		{
			name: "applies styling to in-config but not installed tool",
			tableView: `HEADER LINE
-----------
● kubernetes  kubectl   github_release`,
			rows: []toolRow{
				{status: "●", isInstalled: false, isInConfig: true},
			},
			checkOutput: func(t *testing.T, output string) {
				// Should still contain the header and data.
				assert.Contains(t, output, "HEADER LINE")
				assert.Contains(t, output, "kubernetes")
			},
		},
		{
			name: "handles empty table",
			tableView: `HEADER LINE
-----------`,
			rows: []toolRow{},
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "HEADER LINE")
			},
		},
		{
			name: "handles multiple rows",
			tableView: `HEADER LINE
-----------
● hashicorp    terraform     github_release
  kubernetes   kubectl       github_release
● microsoft    azure-cli     http`,
			rows: []toolRow{
				{status: "●", isInstalled: true, isInConfig: true},
				{status: " ", isInstalled: false, isInConfig: false},
				{status: "●", isInstalled: false, isInConfig: true},
			},
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "hashicorp")
				assert.Contains(t, output, "kubernetes")
				assert.Contains(t, output, "microsoft")
			},
		},
		{
			name: "handles empty lines",
			tableView: `HEADER LINE
-----------

● hashicorp  terraform   github_release

`,
			rows: []toolRow{
				{status: "●", isInstalled: true, isInConfig: true},
			},
			checkOutput: func(t *testing.T, output string) {
				// Should handle empty lines gracefully.
				assert.NotEmpty(t, output)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := renderTableWithConditionalStyling(tt.tableView, tt.rows)
			tt.checkOutput(t, output)
		})
	}
}

// TestRenderTableWithConditionalStyling_SearchRows tests styling with searchRow slice.
func TestRenderTableWithConditionalStyling_SearchRows(t *testing.T) {
	tests := []struct {
		name        string
		tableView   string
		rows        []searchRow
		checkOutput func(t *testing.T, output string)
	}{
		{
			name: "applies styling to installed search result",
			tableView: `HEADER LINE
-----------
● hashicorp  terraform   v1.5.0  github_release`,
			rows: []searchRow{
				{status: "●", isInstalled: true, isInConfig: true},
			},
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "HEADER LINE")
				assert.Contains(t, output, "hashicorp")
			},
		},
		{
			name: "handles empty search results",
			tableView: `HEADER LINE
-----------`,
			rows: []searchRow{},
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "HEADER LINE")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := renderTableWithConditionalStyling(tt.tableView, tt.rows)
			tt.checkOutput(t, output)
		})
	}
}

// TestRenderTableWithConditionalStyling_LineHandling tests edge cases in line handling.
func TestRenderTableWithConditionalStyling_LineHandling(t *testing.T) {
	// Test that header and border lines are not modified.
	tableView := "HEADER\n------\ndata"
	rows := []toolRow{{status: " ", isInstalled: false, isInConfig: false}}

	output := renderTableWithConditionalStyling(tableView, rows)
	lines := strings.Split(output, "\n")

	// Header should be unchanged.
	assert.Equal(t, "HEADER", lines[0])
	// Border should be unchanged.
	assert.Equal(t, "------", lines[1])
}
