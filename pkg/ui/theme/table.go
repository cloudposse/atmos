package theme

import (
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
	Style        TableStyle      // The table style to use
	ShowBorders  bool            // Override for borders (when true, forces borders regardless of Style)
	ShowHeader   bool            // Show header separator (default true for Minimal)
	BorderStyle  lipgloss.Border // Border style when borders are shown
	Styles       *StyleSet       // Reference to theme styles
	StyleFunc    table.StyleFunc // Optional custom style function
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
		// No borders except header separator
		t = t.BorderTop(false).
			BorderBottom(false).
			BorderLeft(false).
			BorderRight(false).
			BorderColumn(false).
			BorderRow(false).
			BorderHeader(true) // Keep header separator

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
			BorderStyle(config.Styles.TableBorder)
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

// CreateThemedTable creates a table with theme-aware styling for the list themes command.
func CreateThemedTable(headers []string, rows [][]string, activeTheme string) string {
	styles := GetCurrentStyles()
	
	config := TableConfig{
		Style:       TableStyleMinimal,
		ShowBorders: false,
		ShowHeader:  true,
		Styles:      styles,
		StyleFunc: func(row, col int) lipgloss.Style {
			style := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
			
			// Header row styling
			if row == -1 {
				return style.Inherit(styles.TableHeader)
			}
			
			// Regular row styling with special cases
			if row >= 0 && row < len(rows) {
				rowData := rows[row]
				
				// Check if this is the active theme row (has "> " indicator)
				isActive := len(rowData) > 0 && rowData[0] == "> "
				
				switch col {
				case 0: // Active indicator column
					if isActive {
						return style.Inherit(styles.Selected)
					}
					return style
					
				case 1: // Name column (may contain ★ for recommended)
					if isActive {
						return style.Inherit(styles.TableActive)
					}
					// Check if name contains the star for recommended themes
					if len(rowData) > 1 && len(rowData[1]) > 0 {
						if rowData[1][len(rowData[1])-1:] == "★" {
							return style.Inherit(styles.TableSpecial)
						}
					}
					return style.Inherit(styles.TableRow)
					
				case 2: // Type column (Dark/Light)
					if len(rowData) > 2 {
						if rowData[2] == "Dark" {
							return style.Inherit(styles.TableDarkType)
						} else if rowData[2] == "Light" {
							return style.Inherit(styles.TableLightType)
						}
					}
					return style.Inherit(styles.TableRow)
					
				case 3: // Source column
					return style.Inherit(styles.TableRow)
					
				default:
					return style.Inherit(styles.TableRow)
				}
			}
			
			return style.Inherit(styles.TableRow)
		},
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