package toolchain

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"golang.org/x/term"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Table row data structure.
type toolRow struct {
	alias       string
	registry    string
	binary      string
	version     string
	status      string
	installDate string
	size        string
	isDefault   bool
	isInstalled bool
}

// RunList prints a table of tools from .tool-versions, marking installed/default versions.
func RunList() error {
	defer perf.Track(nil, "toolchain.RunList")()

	installer := NewInstaller()
	toolVersionsFile := GetToolVersionsFilePath()

	// Load tool versions from file.
	toolVersions, err := LoadToolVersions(toolVersionsFile)
	if err != nil {
		// Handle missing file gracefully with a helpful message.
		if os.IsNotExist(err) {
			message := "# No Configuration Found\n\n" +
				fmt.Sprintf("No `.tool-versions` file found at: `%s`\n\n", toolVersionsFile) +
				"## To get started:\n\n" +
				"Add a tool to automatically create your configuration:\n\n" +
				"```shell\n" +
				"atmos toolchain add terraform@1.6.0\n" +
				"```\n\n" +
				"Then list your tools:\n\n" +
				"```shell\n" +
				"atmos toolchain list\n" +
				"```\n"
			_ = ui.MarkdownMessage(message)
			return nil
		}
		return fmt.Errorf("failed to load .tool-versions: %w", err)
	}

	if len(toolVersions.Tools) == 0 {
		_ = ui.Writeln("No tools configured in .tool-versions file")
		return nil
	}

	var rows []toolRow

	// Process each tool and its versions
	for toolName, versions := range toolVersions.Tools {
		if len(versions) == 0 {
			continue
		}

		// Use existing infrastructure to resolve tool
		resolvedKey, version, found := LookupToolVersion(toolName, toolVersions, installer.resolver)
		if !found {
			_ = ui.Warningf("Could not resolve tool '%s'", toolName)
			continue
		}

		// Get owner/repo from resolved key
		owner, repo, err := installer.resolver.Resolve(resolvedKey)
		if err != nil {
			_ = ui.Warningf("Could not resolve owner/repo for '%s': %v", resolvedKey, err)
			continue
		}

		// No aliases - toolName is what's in .tool-versions
		alias := ""
		if resolvedKey != toolName {
			// If resolved key is different from tool name, tool name might be an alias
			alias = toolName
		}

		// Binary name defaults to repo name
		binaryName := repo

		// Check installation status
		binaryPath, err := installer.FindBinaryPath(owner, repo, version)
		isInstalled := err == nil

		// Debug: log the binary path
		if isInstalled {
			log.Debug("Found binary path", "path", binaryPath, "tool", toolName, "version", version)
		}

		// Build row data
		status := "✗"
		installDate := "N/A"
		size := "N/A"

		if isInstalled {
			status = "✓"
			if fileInfo, err := os.Stat(binaryPath); err == nil {
				size = formatFileSize(fileInfo.Size())
				installDate = fileInfo.ModTime().Format("2006-01-02 15:04")
				// Debug: log the file size
				log.Debug("File size calculated", "path", binaryPath, "size", size, "raw_size", fileInfo.Size())
			} else {
				// Debug: log the error
				log.Debug("Failed to get file info", "path", binaryPath, "error", err)
				size = "N/A"
				installDate = "N/A"
			}
		}

		rows = append(rows, toolRow{
			alias:       alias,
			registry:    fmt.Sprintf("%s/%s", owner, repo),
			binary:      binaryName,
			version:     version,
			status:      status,
			installDate: installDate,
			size:        size,
			isDefault:   true, // First version is default
			isInstalled: isInstalled,
		})

		// Add additional versions if they exist
		for i := 1; i < len(versions); i++ {
			version := versions[i]
			binaryPath, err := installer.FindBinaryPath(owner, repo, version)
			isInstalled := err == nil

			// Debug: log the binary path
			if isInstalled {
				log.Debug("Found binary path", "path", binaryPath, "tool", toolName, "version", version)
			}

			status := "✗"
			installDate := "N/A"
			size := "N/A"

			if isInstalled {
				status = "✓"
				if fileInfo, err := os.Stat(binaryPath); err == nil {
					size = formatFileSize(fileInfo.Size())
					installDate = fileInfo.ModTime().Format("2006-01-02 15:04")
					// Debug: log the file size
					log.Debug("File size calculated", "path", binaryPath, "size", size, "raw_size", fileInfo.Size())
				} else {
					// Debug: log the error
					log.Debug("Failed to get file info", "path", binaryPath, "error", err)
					size = "N/A"
					installDate = "N/A"
				}
			}

			rows = append(rows, toolRow{
				alias:       alias,
				registry:    fmt.Sprintf("%s/%s", owner, repo),
				binary:      binaryName,
				version:     version,
				status:      status,
				installDate: installDate,
				size:        size,
				isDefault:   false,
				isInstalled: isInstalled,
			})
		}
	}

	// Sort rows by registry, then by semantic version
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].registry != rows[j].registry {
			return rows[i].registry < rows[j].registry
		}
		// If same registry, sort by version (newest first)
		return rows[i].version > rows[j].version
	})

	// Get terminal width
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width == 0 {
		width = 120 // fallback width
	}

	// Calculate optimal column widths based on content
	aliasWidth := 0
	registryWidth := 0
	binaryWidth := 0
	versionWidth := 0
	statusWidth := 2 // Just enough for ✓/✗ symbols
	installDateWidth := 0
	sizeWidth := 0

	// Find the maximum width needed for each column
	for _, row := range rows {
		if len(row.alias) > aliasWidth {
			aliasWidth = len(row.alias)
		}
		if len(row.registry) > registryWidth {
			registryWidth = len(row.registry)
		}
		if len(row.binary) > binaryWidth {
			binaryWidth = len(row.binary)
		}
		if len(row.version) > versionWidth {
			versionWidth = len(row.version)
		}
		if len(row.installDate) > installDateWidth {
			installDateWidth = len(row.installDate)
		}
		if len(row.size) > sizeWidth {
			sizeWidth = len(row.size)
		}
		// Debug: log the size width calculation
		log.Debug("Size width calculation", "row_size", row.size, "row_size_len", len(row.size), "current_size_width", sizeWidth)
	}

	// Ensure columns are never narrower than their headers
	headerAliasWidth := len("ALIAS")
	headerRegistryWidth := len("REGISTRY")
	headerBinaryWidth := len("BINARY")
	headerVersionWidth := len("VERSION")
	headerStatusWidth := len("INSTALLED")
	headerInstallDateWidth := len("INSTALL DATE")
	headerSizeWidth := len("SIZE")

	if aliasWidth < headerAliasWidth {
		aliasWidth = headerAliasWidth
	}
	if registryWidth < headerRegistryWidth {
		registryWidth = headerRegistryWidth
	}
	if binaryWidth < headerBinaryWidth {
		binaryWidth = headerBinaryWidth
	}
	if versionWidth < headerVersionWidth {
		versionWidth = headerVersionWidth
	}
	if statusWidth < headerStatusWidth {
		statusWidth = headerStatusWidth
	}
	if installDateWidth < headerInstallDateWidth {
		installDateWidth = headerInstallDateWidth
	}
	if sizeWidth < headerSizeWidth {
		sizeWidth = headerSizeWidth
	}

	// Add buffer (2 spaces on each side)
	aliasWidth += 4
	registryWidth += 4
	binaryWidth += 4
	versionWidth += 4
	installDateWidth += 4
	sizeWidth += 4

	// Calculate total width needed
	totalNeededWidth := aliasWidth + registryWidth + binaryWidth + versionWidth + statusWidth + installDateWidth + sizeWidth

	// Debug: log the width calculation
	log.Debug("Width calculation", "alias", aliasWidth, "registry", registryWidth, "binary", binaryWidth, "version", versionWidth, "status", statusWidth, "installDate", installDateWidth, "size", sizeWidth, "total", totalNeededWidth, "terminal", width)

	// If screen is narrow, truncate columns proportionally
	if totalNeededWidth > width {
		excess := totalNeededWidth - width

		// Truncate proportionally, but keep minimums
		registryReduce := min(excess*3/10, registryWidth-8)
		aliasReduce := min(excess*2/10, aliasWidth-6)
		binaryReduce := min(excess*1/10, binaryWidth-6)
		versionReduce := min(excess*2/10, versionWidth-8)
		installDateReduce := min(excess*1/10, installDateWidth-12)
		sizeReduce := min(excess*1/10, sizeWidth-8)

		registryWidth -= registryReduce
		aliasWidth -= aliasReduce
		binaryWidth -= binaryReduce
		versionWidth -= versionReduce
		installDateWidth -= installDateReduce
		sizeWidth -= sizeReduce
	}

	// Create table columns with calculated widths
	columns := []table.Column{
		{Title: "ALIAS", Width: aliasWidth},
		{Title: "REGISTRY", Width: registryWidth},
		{Title: "BINARY", Width: binaryWidth},
		{Title: "VERSION", Width: versionWidth},
		{Title: "INSTALLED", Width: statusWidth},
		{Title: "INSTALL DATE", Width: installDateWidth},
		{Title: "SIZE", Width: sizeWidth},
	}

	// Debug: log the final column widths
	log.Debug("Final column widths", "alias", aliasWidth, "registry", registryWidth, "binary", binaryWidth, "version", versionWidth, "status", statusWidth, "installDate", installDateWidth, "size", sizeWidth, "total_width", totalNeededWidth, "terminal_width", width)

	// Convert rows to table format - use plain text for proper sizing
	var tableRows []table.Row
	for i, row := range rows {
		// Debug: log the row data
		log.Debug("Creating table row", "index", i, "alias", row.alias, "registry", row.registry, "binary", row.binary, "version", row.version, "status", row.status, "installDate", row.installDate, "size", row.size, "size_len", len(row.size))

		// Check if size is empty or nil
		if row.size == "" {
			log.Debug("WARNING: Empty size for row", "index", i, "tool", row.alias)
		}

		tableRows = append(tableRows, table.Row{
			row.alias,
			row.registry,
			row.binary,
			row.version,
			row.status,
			row.installDate,
			row.size,
		})
	}

	// Create and configure table
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(tableRows),
		table.WithFocused(false), // Non-interactive
		table.WithHeight(len(tableRows)+1),
	)

	// Debug: log table configuration
	log.Debug("Table configuration", "num_columns", len(columns), "num_rows", len(tableRows), "height", len(tableRows))

	// Set table width to use the calculated width
	// t.SetWidth(totalNeededWidth) // Comment out to see if this is causing the issue

	// Debug: log the table width
	log.Debug("Table width set", "width", totalNeededWidth)

	// Create custom styles for different states
	defaultStyle := table.DefaultStyles()
	defaultStyle.Header = defaultStyle.Header.Bold(true)
	defaultStyle.Cell = defaultStyle.Cell.PaddingLeft(1).PaddingRight(1)
	defaultStyle.Selected = defaultStyle.Cell

	// Create styles for uninstalled tools (gray)
	uninstalledStyle := table.DefaultStyles()
	uninstalledStyle.Header = uninstalledStyle.Header.Bold(true)
	uninstalledStyle.Cell = uninstalledStyle.Cell.PaddingLeft(1).PaddingRight(1).Foreground(lipgloss.Color("240"))
	uninstalledStyle.Selected = uninstalledStyle.Cell

	// Set the default styles
	t.SetStyles(defaultStyle)

	// Print the table with conditional styling
	// fmt.Println(renderTableWithConditionalStyling(t, rows, defaultStyle, uninstalledStyle))
	// fmt.Println(t.View())

	// Print the table with conditional styling
	fmt.Println(renderTableWithConditionalStyling(t, rows, defaultStyle, uninstalledStyle))

	return nil
}

// renderTableWithConditionalStyling renders the table with proper conditional styling.
func renderTableWithConditionalStyling(t table.Model, rows []toolRow, defaultStyle, uninstalledStyle table.Styles) string {
	// Get the base table view
	tableView := t.View()

	// Split into lines
	lines := strings.Split(tableView, "\n")

	// Apply conditional styling to each row
	for i, line := range lines {
		if i == 0 {
			// Header line - keep as is
			continue
		}

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Apply styling based on row data (adjust index for header)
		rowIndex := i - 1
		if rowIndex < len(rows) {
			rowData := rows[rowIndex]
			if !rowData.isInstalled {
				// Gray for uninstalled - preserve the exact line structure
				lines[i] = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(line)
			}
			// Default and installed non-default stay white (default color)
		}
	}

	return strings.Join(lines, "\n")
}

// min returns the smaller of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatFileSize formats file size in human readable format.
func formatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
