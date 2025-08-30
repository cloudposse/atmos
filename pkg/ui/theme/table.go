package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// TableStyle represents different table design styles.
type TableStyle int

const (
	// TableStyleBordered shows full borders around and within the table.
	TableStyleBordered TableStyle = iota
	// TableStyleMinimal shows no borders except header separator.
	TableStyleMinimal
	// TableStylePlain shows no borders at all.
	TableStylePlain
)

// TableConfig defines the configuration for table rendering.
type TableConfig struct {
	Style       TableStyle      // The table style to use
	ShowBorders bool            // Override for borders (when true, forces borders regardless of Style)
	ShowHeader  bool            // Show header separator (default true for Minimal)
	BorderStyle lipgloss.Border // Border style when borders are shown
	Styles      *StyleSet       // Reference to theme styles
	StyleFunc   table.StyleFunc // Optional custom style function
}

// DefaultTableConfig returns a default table configuration with minimal style.
func DefaultTableConfig() TableConfig {
	return TableConfig{
		Style:       TableStyleMinimal,
		ShowBorders: false,
		ShowHeader:  true,
		BorderStyle: lipgloss.NormalBorder(),
		Styles:      GetCurrentStyles(),
	}
}

// CreateTable creates a styled table based on the configuration.
func CreateTable(config *TableConfig, headers []string, rows [][]string) string {
	t := table.New()

	// Apply table style based on configuration
	switch config.Style {
	case TableStyleBordered:
		// Full borders
		t = t.Border(config.BorderStyle).
			BorderStyle(config.Styles.TableBorder)

	case TableStyleMinimal:
		// No borders except header separator (if ShowHeader is true)
		t = t.BorderTop(false).
			BorderBottom(false).
			BorderLeft(false).
			BorderRight(false).
			BorderColumn(false).
			BorderRow(false).
			BorderHeader(config.ShowHeader) // Honor ShowHeader setting

	case TableStylePlain:
		// No borders at all
		t = t.BorderTop(false).
			BorderBottom(false).
			BorderLeft(false).
			BorderRight(false).
			BorderColumn(false).
			BorderRow(false).
			BorderHeader(false)
	}

	// Override with ShowBorders if explicitly set
	if config.ShowBorders && config.Style != TableStyleBordered {
		t = t.Border(config.BorderStyle).
			BorderStyle(config.Styles.TableBorder).
			BorderTop(true).
			BorderBottom(true).
			BorderLeft(true).
			BorderRight(true).
			BorderColumn(true).
			BorderRow(true).
			BorderHeader(true)
	}

	// Apply style function
	if config.StyleFunc != nil {
		t = t.StyleFunc(config.StyleFunc)
	} else {
		// Use default style function based on theme
		t = t.StyleFunc(func(row, col int) lipgloss.Style {
			style := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)

			if row == -1 { // Header row
				return style.Inherit(config.Styles.TableHeader)
			}
			return style.Inherit(config.Styles.TableRow)
		})
	}

	// Set headers and rows
	t = t.Headers(headers...).Rows(rows...)

	return t.String()
}

// isActiveRow checks if the row represents an active theme.
func isActiveRow(rowData []string) bool {
	return len(rowData) > 0 && rowData[0] == "> "
}

// isRecommendedTheme checks if the theme name contains a star indicator.
func isRecommendedTheme(name string) bool {
	return strings.HasSuffix(name, "★")
}

// getActiveColumnStyle returns the style for the active indicator column.
func getActiveColumnStyle(isActive bool, styles *StyleSet) lipgloss.Style {
	baseStyle := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
	if styles == nil {
		return baseStyle
	}
	if isActive {
		return baseStyle.Inherit(styles.Selected)
	}
	return baseStyle
}

// getNameColumnStyle returns the style for the name column.
func getNameColumnStyle(rowData []string, isActive bool, styles *StyleSet) lipgloss.Style {
	baseStyle := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
	if styles == nil {
		return baseStyle
	}
	if isActive {
		return baseStyle.Inherit(styles.TableActive)
	}
	if len(rowData) > 1 && isRecommendedTheme(rowData[1]) {
		return baseStyle.Inherit(styles.TableSpecial)
	}
	return baseStyle.Inherit(styles.TableRow)
}

// getTypeColumnStyle returns the style for the type column.
func getTypeColumnStyle(rowData []string, styles *StyleSet) lipgloss.Style {
	baseStyle := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
	if styles == nil {
		return baseStyle
	}
	if len(rowData) > 2 {
		switch rowData[2] {
		case "Dark":
			return baseStyle.Inherit(styles.TableDarkType)
		case "Light":
			return baseStyle.Inherit(styles.TableLightType)
		}
	}
	return baseStyle.Inherit(styles.TableRow)
}

// getCellStyle determines the appropriate style for a table cell.
func getCellStyle(col int, rowData []string, isActive bool, styles *StyleSet) lipgloss.Style {
	switch col {
	case 0: // Active indicator column
		return getActiveColumnStyle(isActive, styles)
	case 1: // Name column (may contain ★ for recommended)
		return getNameColumnStyle(rowData, isActive, styles)
	case 2: // Type column (Dark/Light)
		return getTypeColumnStyle(rowData, styles)
	default: // Source column and others
		baseStyle := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
		if styles == nil {
			return baseStyle
		}
		return baseStyle.Inherit(styles.TableRow)
	}
}

// createTableStyleFunc returns a styling function for the table.
func createTableStyleFunc(rows [][]string, styles *StyleSet) func(int, int) lipgloss.Style {
	return func(row, col int) lipgloss.Style {
		// Nil safety check
		if styles == nil {
			return lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
		}

		baseStyle := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)

		// Header row styling
		if row == -1 {
			return baseStyle.Inherit(styles.TableHeader)
		}

		// Regular row styling
		if row >= 0 && row < len(rows) {
			rowData := rows[row]
			isActive := isActiveRow(rowData)
			return getCellStyle(col, rowData, isActive, styles)
		}

		return baseStyle.Inherit(styles.TableRow)
	}
}

// CreateThemedTable creates a table with theme-aware styling for the list themes command.
func CreateThemedTable(headers []string, rows [][]string) string {
	styles := GetCurrentStyles()

	config := TableConfig{
		Style:       TableStyleMinimal,
		ShowBorders: false,
		ShowHeader:  true,
		Styles:      styles,
		StyleFunc:   createTableStyleFunc(rows, styles),
	}

	return CreateTable(&config, headers, rows)
}

// CreateMinimalTable creates a table with minimal styling (header separator only).
func CreateMinimalTable(headers []string, rows [][]string) string {
	config := DefaultTableConfig()
	config.Style = TableStyleMinimal
	return CreateTable(&config, headers, rows)
}

// CreateBorderedTable creates a table with full borders.
func CreateBorderedTable(headers []string, rows [][]string) string {
	config := DefaultTableConfig()
	config.Style = TableStyleBordered
	config.BorderStyle = lipgloss.NormalBorder()
	return CreateTable(&config, headers, rows)
}

// CreatePlainTable creates a table with no borders or separators.
func CreatePlainTable(headers []string, rows [][]string) string {
	config := DefaultTableConfig()
	config.Style = TableStylePlain
	return CreateTable(&config, headers, rows)
}
