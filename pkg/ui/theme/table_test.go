package theme

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultTableConfig(t *testing.T) {
	config := DefaultTableConfig()
	assert.Equal(t, TableStyleMinimal, config.Style)
	assert.False(t, config.ShowBorders)
	assert.True(t, config.ShowHeader)
	assert.NotNil(t, config.BorderStyle)
	assert.NotNil(t, config.Styles)
}

func TestCreateTable_MinimalStyle(t *testing.T) {
	InitializeStylesFromTheme("atmos")
	config := DefaultTableConfig()
	config.Style = TableStyleMinimal

	headers := []string{"Name", "Type"}
	rows := [][]string{
		{"component1", "terraform"},
		{"component2", "helmfile"},
	}

	output := CreateTable(&config, headers, rows)
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "Name")
	assert.Contains(t, output, "component1")
}

func TestCreateTable_BorderedStyle(t *testing.T) {
	InitializeStylesFromTheme("atmos")
	config := DefaultTableConfig()
	config.Style = TableStyleBordered

	headers := []string{"Name", "Type"}
	rows := [][]string{
		{"component1", "terraform"},
	}

	output := CreateTable(&config, headers, rows)
	assert.NotEmpty(t, output)
	// Bordered table should have border characters
	assert.True(t, strings.Contains(output, "─") || strings.Contains(output, "-"))
}

func TestCreateTable_PlainStyle(t *testing.T) {
	InitializeStylesFromTheme("atmos")
	config := DefaultTableConfig()
	config.Style = TableStylePlain

	headers := []string{"Name", "Type"}
	rows := [][]string{
		{"component1", "terraform"},
	}

	output := CreateTable(&config, headers, rows)
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "component1")
}

func TestCreateMinimalTable(t *testing.T) {
	InitializeStylesFromTheme("atmos")

	headers := []string{"Name", "Status"}
	rows := [][]string{
		{"test1", "active"},
		{"test2", "inactive"},
	}

	output := CreateMinimalTable(headers, rows)
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "Name")
	assert.Contains(t, output, "test1")
	assert.Contains(t, output, "test2")
}

func TestCreateBorderedTable(t *testing.T) {
	InitializeStylesFromTheme("atmos")

	headers := []string{"Name", "Status"}
	rows := [][]string{
		{"test1", "active"},
	}

	output := CreateBorderedTable(headers, rows)
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "test1")
}

func TestCreatePlainTable(t *testing.T) {
	InitializeStylesFromTheme("atmos")

	headers := []string{"Name", "Status"}
	rows := [][]string{
		{"test1", "active"},
	}

	output := CreatePlainTable(headers, rows)
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "test1")
}

func TestCreateThemedTable(t *testing.T) {
	InitializeStylesFromTheme("atmos")

	headers := []string{"Active", "Name", "Type", "Source"}
	rows := [][]string{
		{"> ", "dracula★", "Dark", "https://github.com/dracula"},
		{"", "nord", "Dark", "https://github.com/nord"},
	}

	output := CreateThemedTable(headers, rows)
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "dracula")
	assert.Contains(t, output, "nord")
}

func TestIsActiveRow(t *testing.T) {
	tests := []struct {
		name     string
		rowData  []string
		expected bool
	}{
		{
			name:     "active row",
			rowData:  []string{"●", "theme1"},
			expected: true,
		},
		{
			name:     "inactive row",
			rowData:  []string{"", "theme2"},
			expected: false,
		},
		{
			name:     "empty row",
			rowData:  []string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isActiveRow(tt.rowData)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsRecommendedTheme(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "recommended theme with star",
			input:    "★",
			expected: true,
		},
		{
			name:     "theme without star",
			input:    "",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// isRecommendedTheme now expects []string (row data), so wrap the input
			rowData := []string{tt.input}
			result := isRecommendedTheme(rowData)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetActiveColumnStyle(t *testing.T) {
	InitializeStylesFromTheme("atmos")
	styles := GetCurrentStyles()

	// Test active (not recommended)
	activeStyle := getActiveColumnStyle(true, false, styles)
	assert.NotNil(t, activeStyle)

	// Test recommended (not active)
	recommendedStyle := getActiveColumnStyle(false, true, styles)
	assert.NotNil(t, recommendedStyle)

	// Test inactive (not recommended)
	inactiveStyle := getActiveColumnStyle(false, false, styles)
	assert.NotNil(t, inactiveStyle)

	// Test with nil styles
	nilStyle := getActiveColumnStyle(true, false, nil)
	assert.NotNil(t, nilStyle)
}

func TestGetNameColumnStyle(t *testing.T) {
	InitializeStylesFromTheme("atmos")
	styles := GetCurrentStyles()

	tests := []struct {
		name     string
		rowData  []string
		isActive bool
	}{
		{
			name:     "active row",
			rowData:  []string{"> ", "theme1"},
			isActive: true,
		},
		{
			name:     "recommended theme",
			rowData:  []string{"", "theme1★"},
			isActive: false,
		},
		{
			name:     "regular theme",
			rowData:  []string{"", "theme1"},
			isActive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			style := getNameColumnStyle(tt.rowData, tt.isActive, styles)
			assert.NotNil(t, style)
		})
	}
}

func TestGetTypeColumnStyle(t *testing.T) {
	InitializeStylesFromTheme("atmos")
	styles := GetCurrentStyles()

	tests := []struct {
		name    string
		rowData []string
	}{
		{
			name:    "dark theme",
			rowData: []string{"", "theme1", "Dark"},
		},
		{
			name:    "light theme",
			rowData: []string{"", "theme1", "Light"},
		},
		{
			name:    "other type",
			rowData: []string{"", "theme1", "Other"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			style := getTypeColumnStyle(tt.rowData, styles)
			assert.NotNil(t, style)
		})
	}
}

func TestGetCellStyle(t *testing.T) {
	InitializeStylesFromTheme("atmos")
	styles := GetCurrentStyles()

	rowData := []string{"> ", "dracula★", "Dark", "source"}

	// Test each column
	col0Style := getCellStyle(0, rowData, true, styles)
	assert.NotNil(t, col0Style)

	col1Style := getCellStyle(1, rowData, true, styles)
	assert.NotNil(t, col1Style)

	col2Style := getCellStyle(2, rowData, false, styles)
	assert.NotNil(t, col2Style)

	col3Style := getCellStyle(3, rowData, false, styles)
	assert.NotNil(t, col3Style)
}

func TestCreateTableStyleFunc(t *testing.T) {
	InitializeStylesFromTheme("atmos")
	styles := GetCurrentStyles()

	rows := [][]string{
		{"> ", "dracula★", "Dark", "source1"},
		{"", "nord", "Dark", "source2"},
	}

	styleFunc := createTableStyleFunc(rows, styles)
	require.NotNil(t, styleFunc)

	// Test header row
	headerStyle := styleFunc(-1, 0)
	assert.NotNil(t, headerStyle)

	// Test data rows
	row0Style := styleFunc(0, 0)
	assert.NotNil(t, row0Style)

	row1Style := styleFunc(1, 1)
	assert.NotNil(t, row1Style)

	// Test out of bounds
	outOfBoundsStyle := styleFunc(999, 0)
	assert.NotNil(t, outOfBoundsStyle)
}

func TestCreateTableStyleFunc_NilStyles(t *testing.T) {
	rows := [][]string{
		{"test"},
	}

	styleFunc := createTableStyleFunc(rows, nil)
	require.NotNil(t, styleFunc)

	// Should return a basic style without crashing
	style := styleFunc(0, 0)
	assert.NotNil(t, style)
}

func TestTableConfig_WithCustomStyleFunc(t *testing.T) {
	InitializeStylesFromTheme("atmos")

	customStyleFunc := func(row, col int) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	}

	config := TableConfig{
		Style:       TableStyleMinimal,
		ShowBorders: false,
		ShowHeader:  true,
		BorderStyle: lipgloss.NormalBorder(),
		Styles:      GetCurrentStyles(),
		StyleFunc:   customStyleFunc,
	}

	headers := []string{"Name"}
	rows := [][]string{{"test"}}

	output := CreateTable(&config, headers, rows)
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "test")
}
