package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"

	"github.com/cloudposse/atmos/experiments/init/embeds"
	"github.com/cloudposse/atmos/experiments/init/internal/config"
	"github.com/cloudposse/atmos/experiments/init/internal/filesystem"
	"github.com/cloudposse/atmos/experiments/init/internal/templating"
)

// FileSkippedError indicates that a file was intentionally skipped
type FileSkippedError struct {
	Path         string
	RenderedPath string
}

func (e *FileSkippedError) Error() string {
	return fmt.Sprintf("file skipped: %s (rendered to: %s)", e.Path, e.RenderedPath)
}

// truncateString truncates a string to the specified length and adds "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// spinnerModel wraps the spinner for tea.Model compatibility
type spinnerModel struct {
	spinner spinner.Model
	message string
}

func (m spinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		default:
			return m, nil
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m spinnerModel) View() string {
	return fmt.Sprintf("\r%s %s", m.spinner.View(), m.message)
}

// InitUI handles the user interface for the init command
type InitUI struct {
	checkmark    string
	xMark        string
	grayStyle    lipgloss.Style
	successStyle lipgloss.Style
	errorStyle   lipgloss.Style
	output       strings.Builder
	processor    *templating.Processor
}

// NewInitUI creates a new InitUI instance
func NewInitUI() *InitUI {
	return &InitUI{
		checkmark:    "✓",
		xMark:        "✗",
		grayStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		successStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		errorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		output:       strings.Builder{},
		processor:    templating.NewProcessor(),
	}
}

// SetThreshold sets the threshold for merge operations
func (ui *InitUI) SetThreshold(thresholdPercent int) {
	ui.processor.SetMaxChanges(thresholdPercent)
}

// GetTerminalWidth returns the current terminal width with a fallback
func (ui *InitUI) GetTerminalWidth() int {
	width, _, err := term.GetSize(uintptr(os.Stdout.Fd()))
	if err != nil {
		return 80 // fallback width
	}
	return width
}

// writeOutput writes to the output buffer instead of using fmt.Printf
func (ui *InitUI) writeOutput(format string, args ...interface{}) {
	ui.output.WriteString(fmt.Sprintf(format, args...))
}

// flushOutput writes the accumulated output to stdout and clears the buffer
func (ui *InitUI) flushOutput() {
	fmt.Print(ui.output.String())
	ui.output.Reset()
}

// Execute runs the initialization process with UI
func (ui *InitUI) Execute(embedsConfig embeds.Configuration, targetPath string, force, update, useDefaults bool, cmdTemplateValues map[string]interface{}) error {
	// Early validation: check if target directory exists and handle appropriately
	if err := filesystem.ValidateTargetDirectory(targetPath, force, update); err != nil {
		return err
	}

	ui.writeOutput("Generating %s in %s\n\n", embedsConfig.Name, targetPath)

	// Check if this configuration has a scaffold.yaml file (project schema)
	if embeds.HasScaffoldConfig(embedsConfig.Files) {
		return ui.executeWithSetup(embedsConfig, targetPath, force, update, useDefaults, cmdTemplateValues)
	}

	// For templates without scaffold.yaml, use command-line values if provided
	if len(cmdTemplateValues) > 0 {
		return ui.executeWithCommandValues(embedsConfig, targetPath, force, update, cmdTemplateValues)
	}

	// Load user configuration and prompt if needed
	userConfig, err := config.LoadUserConfiguration()
	if err != nil {
		return fmt.Errorf("failed to load user configuration: %w", err)
	}

	return ui.executeWithUserConfig(embedsConfig, targetPath, force, update, userConfig)
}

// executeWithCommandValues processes files using command-line template values
func (ui *InitUI) executeWithCommandValues(embedsConfig embeds.Configuration, targetPath string, force, update bool, cmdTemplateValues map[string]interface{}) error {
	// For now, use the existing processFile method but this should be refactored
	// to use the templating processor properly
	var successCount, errorCount int
	for _, file := range embedsConfig.Files {
		// Process the file using the templating processor
		// Convert embeds.File to templating.File
		templatingFile := templating.File{
			Path:        file.Path,
			Content:     file.Content,
			IsTemplate:  file.IsTemplate,
			Permissions: file.Permissions,
		}

		err := ui.processor.ProcessFile(templatingFile, targetPath, force, update, nil, cmdTemplateValues)

		// Display result using proper UI output
		if err != nil {
			// Check if this is a FileSkippedError
			if skipErr, ok := err.(*templating.FileSkippedError); ok {
				// File was intentionally skipped
				ui.writeOutput("  %s %s %s\n",
					ui.grayStyle.Render("•"),
					skipErr.Path,
					ui.grayStyle.Render("(skipped)"))
			} else {
				// Actual error occurred
				errorCount++
				ui.writeOutput("  %s %s %s\n",
					ui.errorStyle.Render(ui.xMark),
					file.Path,
					ui.grayStyle.Render(fmt.Sprintf("(error: %v)", err)))
			}
		} else {
			successCount++
			ui.writeOutput("  %s %s\n",
				ui.successStyle.Render(ui.checkmark),
				file.Path)
		}
	}

	// Print summary
	ui.writeOutput("\n")
	if errorCount > 0 {
		ui.writeOutput("Initialized %d files. Failed to initialize %d files.\n", successCount, errorCount)
		ui.flushOutput()
		return fmt.Errorf("failed to initialize %d files", errorCount)
	} else {
		ui.writeOutput("Initialized %d files.\n", successCount)
	}

	// Flush all output before rendering README
	ui.flushOutput()

	// Only render README if all files were successful
	if embedsConfig.README != "" {
		if err := ui.renderREADME(embedsConfig.README, targetPath); err != nil {
			return err
		}
	}

	return nil
}

// executeWithUserConfig processes files using user configuration
func (ui *InitUI) executeWithUserConfig(embedsConfig embeds.Configuration, targetPath string, force, update bool, userConfig *config.Config) error {
	// For now, use the existing processFileWithConfig method but this should be refactored
	// to use the templating processor properly
	var successCount, errorCount int
	for _, file := range embedsConfig.Files {
		// Process the file with user configuration using templating processor
		// Convert embeds.File to templating.File
		templatingFile := templating.File{
			Path:        file.Path,
			Content:     file.Content,
			IsTemplate:  file.IsTemplate,
			Permissions: file.Permissions,
		}

		// Load dynamic user values from the scaffold template directory
		userValues, loadErr := config.LoadUserValues(targetPath)
		if loadErr != nil {
			// If no user values file exists, create empty map
			userValues = make(map[string]interface{})
		}

		err := ui.processor.ProcessFile(templatingFile, targetPath, force, update, nil, userValues)

		// Display result using proper UI output
		if err != nil {
			// Check if this is a FileSkippedError
			if skipErr, ok := err.(*FileSkippedError); ok {
				// File was intentionally skipped
				ui.writeOutput("  %s %s %s\n",
					ui.grayStyle.Render("•"),
					skipErr.Path,
					ui.grayStyle.Render("(skipped)"))
			} else {
				// Actual error occurred
				errorCount++
				ui.writeOutput("  %s %s %s\n",
					ui.errorStyle.Render(ui.xMark),
					file.Path,
					ui.grayStyle.Render(fmt.Sprintf("(error: %v)", err)))
			}
		} else {
			successCount++
			ui.writeOutput("  %s %s\n",
				ui.successStyle.Render(ui.checkmark),
				file.Path)
		}
	}

	// Print summary
	ui.writeOutput("\n")
	if errorCount > 0 {
		ui.writeOutput("Initialized %d files. Failed to initialize %d files.\n", successCount, errorCount)
		ui.flushOutput()
		return fmt.Errorf("failed to initialize %d files", errorCount)
	} else {
		ui.writeOutput("Initialized %d files.\n", successCount)
	}

	// Flush all output before rendering README
	ui.flushOutput()

	// Only render README if all files were successful
	if embedsConfig.README != "" {
		if err := ui.renderREADME(embedsConfig.README, targetPath); err != nil {
			return err
		}
	}

	return nil
}

// RunSetupForm runs the interactive setup form to collect configuration values
// This method can be used by both init and scaffold commands
func (ui *InitUI) RunSetupForm(scaffoldConfig *config.ScaffoldConfig, targetPath string, useDefaults bool, cmdTemplateValues map[string]interface{}) (map[string]interface{}, map[string]string, error) {
	// Load existing user values from the scaffold template directory
	userValues, err := config.LoadUserValues(targetPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load user values: %w", err)
	}

	// Deep merge project defaults with user values
	mergedValues := config.DeepMerge(scaffoldConfig, userValues)

	// Override with command-line config values (highest priority)
	for key, value := range cmdTemplateValues {
		mergedValues[key] = value
	}

	// Track value sources for display
	valueSources := make(map[string]string)

	// Start with all values as defaults
	for key := range scaffoldConfig.Fields {
		if _, exists := mergedValues[key]; exists {
			valueSources[key] = "default"
		}
	}

	// Mark values that came from existing config (scaffold) - these override defaults
	for key := range userValues {
		valueSources[key] = "scaffold"
	}

	// Override with command-line config values (highest priority)
	for key, value := range cmdTemplateValues {
		mergedValues[key] = value
		valueSources[key] = "flag"
	}

	// Debug: Print valueSources map
	fmt.Printf("DEBUG: valueSources map: %+v\n", valueSources)

	// Prompt the user to edit the configuration values unless --use-defaults is specified
	// This allows them to review and modify values from command line, config, or defaults
	if !useDefaults {
		if err := config.PromptForScaffoldConfig(scaffoldConfig, mergedValues); err != nil {
			return nil, nil, fmt.Errorf("failed to prompt for configuration: %w", err)
		}
	}

	// Show configuration summary after any user input
	// Get configuration summary data and display it
	rows, header := config.GetConfigurationSummary(scaffoldConfig, mergedValues, valueSources)

	// Debug: Print valueSources to see what we have
	fmt.Printf("DEBUG: valueSources: %+v\n", valueSources)

	ui.displayConfigurationTable(header, rows)

	// Flush the configuration summary before processing files
	ui.flushOutput()

	return mergedValues, valueSources, nil
}

// executeWithSetup handles any scaffold configuration with interactive prompts
func (ui *InitUI) executeWithSetup(embedsConfig embeds.Configuration, targetPath string, force, update, useDefaults bool, cmdTemplateValues map[string]interface{}) error {
	// Find the scaffold.yaml file in the configuration
	var scaffoldConfigFile *embeds.File
	for i := range embedsConfig.Files {
		if embedsConfig.Files[i].Path == config.ScaffoldConfigFileName {
			scaffoldConfigFile = &embedsConfig.Files[i]
			break
		}
	}

	if scaffoldConfigFile == nil {
		return fmt.Errorf("%s not found in rich-project configuration", config.ScaffoldConfigFileName)
	}

	// Create directory if needed
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Load the scaffold configuration from embedded content (don't write to target folder)
	scaffoldConfig, err := config.LoadScaffoldConfigFromContent(scaffoldConfigFile.Content)
	if err != nil {
		return fmt.Errorf("failed to load scaffold configuration: %w", err)
	}

	// Run the setup form to collect configuration values
	mergedValues, _, err := ui.RunSetupForm(scaffoldConfig, targetPath, useDefaults, cmdTemplateValues)
	if err != nil {
		return fmt.Errorf("failed to run setup form: %w", err)
	}

	// Save the user values with template ID
	if err := config.SaveUserConfig(targetPath, embedsConfig.TemplateID, mergedValues); err != nil {
		return fmt.Errorf("failed to save user values: %w", err)
	}

	// Process each file with rich configuration
	var successCount, errorCount int
	var failedFiles []string
	for _, file := range embedsConfig.Files {
		// Skip the scaffold.yaml as it's only used for schema definition
		if file.Path == config.ScaffoldConfigFileName {
			continue
		}

		// Process the file path as a template first to check if it should be skipped
		renderedPath, pathErr := ui.processor.ProcessTemplate(file.Path, targetPath, scaffoldConfig, mergedValues)
		if pathErr != nil {
			// If path processing fails, use original path
			renderedPath = file.Path
		}

		// Check if the rendered path should be skipped
		if ui.processor.ShouldSkipFile(renderedPath) {
			// File was intentionally skipped
			ui.writeOutput("  %s %s %s\n",
				ui.grayStyle.Render("•"),
				file.Path,
				ui.grayStyle.Render("(skipped)"))
			continue
		}

		// Use the templating processor to handle file processing
		// Convert embeds.File to templating.File
		templatingFile := templating.File{
			Path:        file.Path,
			Content:     file.Content,
			IsTemplate:  file.IsTemplate,
			Permissions: file.Permissions,
		}
		err := ui.processor.ProcessFile(templatingFile, targetPath, force, update, scaffoldConfig, mergedValues)

		// Display result using proper UI output
		if err != nil {
			// Check if this is a FileSkippedError
			if skipErr, ok := err.(*FileSkippedError); ok {
				// File was intentionally skipped
				ui.writeOutput("  %s %s %s\n",
					ui.grayStyle.Render("•"),
					skipErr.Path,
					ui.grayStyle.Render("(skipped)"))
			} else {
				// Actual error occurred
				errorCount++
				failedFiles = append(failedFiles, file.Path)
				ui.writeOutput("  %s %s %s\n",
					ui.errorStyle.Render(ui.xMark),
					renderedPath,
					ui.grayStyle.Render(fmt.Sprintf("(error: %v)", err)))
			}
		} else {
			successCount++
			ui.writeOutput("  %s %s\n",
				ui.successStyle.Render(ui.checkmark),
				renderedPath)
		}
	}

	// Print summary
	ui.writeOutput("\n")
	if errorCount > 0 {
		ui.writeOutput("Generated %d files. Failed to generate %d files.\n", successCount, errorCount)
		// Don't render README if there were errors - flush output and return error immediately
		ui.flushOutput()
		return fmt.Errorf("failed to generate files: %s", strings.Join(failedFiles, ", "))
	} else {
		ui.writeOutput("Generated %d files.\n", successCount)
	}

	// Flush all output before rendering README
	ui.flushOutput()

	// Only render README if all files were successful
	if embedsConfig.README != "" {
		// Process README template with rich configuration
		processedContent, err := ui.processor.ProcessTemplate(embedsConfig.README, targetPath, scaffoldConfig, mergedValues)
		if err != nil {
			return fmt.Errorf("failed to process README template: %w", err)
		}

		// Render the processed content as markdown
		if err := ui.renderMarkdown(processedContent); err != nil {
			return err
		}
	}

	return nil
}

// renderMarkdown renders markdown content using glamour
func (ui *InitUI) renderMarkdown(markdownContent string) error {
	// Create glamour renderer with dynamic terminal width
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(ui.GetTerminalWidth()),
		glamour.WithEmoji(),
	)
	if err != nil {
		return fmt.Errorf("failed to create markdown renderer: %w", err)
	}

	// Render the markdown
	rendered, err := renderer.Render(markdownContent)
	if err != nil {
		return fmt.Errorf("failed to render markdown: %w", err)
	}

	// Display the rendered markdown
	fmt.Println("\n" + rendered)

	return nil
}

// renderREADME renders the README content using glamour
func (ui *InitUI) renderREADME(readmeContent string, targetPath string) error {
	// Process README template
	processedContent, err := ui.processor.ProcessTemplate(readmeContent, targetPath, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to process README template: %w", err)
	}

	// Render the processed content as markdown
	return ui.renderMarkdown(processedContent)
}

// displayConfigurationTable displays configuration data in a formatted table
func (ui *InitUI) displayConfigurationTable(header []string, rows [][]string) {
	// Get terminal width
	width, _, err := term.GetSize(uintptr(os.Stdout.Fd()))
	if err != nil {
		width = 80 // fallback width
	}

	// Calculate table width (leave some margin)
	tableWidth := width - 20

	// Convert rows to table.Row format and apply source colors
	var tableRows []table.Row
	for _, row := range rows {
		// Apply color to source column based on the source value
		source := row[2]
		var coloredSource string
		switch source {
		case "scaffold":
			coloredSource = lipgloss.NewStyle().Foreground(lipgloss.Color("#0000FF")).Render("scaffold") // Blue
		case "flag":
			coloredSource = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("flag") // Red
		default:
			coloredSource = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")).Render("default") // Grey
		}

		// Create new row with colored source
		coloredRow := []string{row[0], row[1], coloredSource}
		tableRows = append(tableRows, table.Row(coloredRow))
	}

	// Calculate column widths based on content
	settingWidth := 12 // Minimum width for setting names
	valueWidth := 45   // Minimum width for values
	sourceWidth := 12  // Minimum width for sources

	// Find the maximum content width for each column
	for _, row := range rows {
		if len(row[0]) > settingWidth {
			settingWidth = len(row[0])
		}
		if len(row[1]) > valueWidth {
			valueWidth = len(row[1])
		}
		if len(row[2]) > sourceWidth {
			sourceWidth = len(row[2])
		}
	}

	// Add some padding to each column
	settingWidth += 2
	valueWidth += 2
	sourceWidth += 2

	// Calculate total table width needed
	totalContentWidth := settingWidth + valueWidth + sourceWidth + 6 // +6 for borders and spacing

	// If content is wider than screen, use content width; otherwise use screen width
	if totalContentWidth > tableWidth {
		tableWidth = totalContentWidth
	}

	// Create table
	t := table.New(
		table.WithColumns([]table.Column{
			{Title: "Setting", Width: settingWidth},
			{Title: "Value", Width: valueWidth},
			{Title: "Source", Width: sourceWidth},
		}),
		table.WithRows(tableRows),
		table.WithWidth(tableWidth),
		table.WithFocused(false),
		table.WithHeight(len(tableRows)+1), // Set explicit height to minimize spacing
	)

	// Style the table with colors
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5F5FD7")). // Purple
		BorderBottom(true).
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")) // White

	// Use a custom style function to apply source-specific colors
	s.Cell = s.Cell.Foreground(lipgloss.Color("#FFFFFF")).Bold(false)

	// Add custom styling for source column
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#FFFFFF")). // White
		Background(lipgloss.Color("#000000"))  // Black

	t.SetStyles(s)

	// Print the table
	ui.writeOutput("\n")
	ui.writeOutput("CONFIGURATION SUMMARY\n")
	ui.writeOutput("\n")
	ui.writeOutput("%s\n", t.View())
	ui.writeOutput("\n")
}

// DisplayTemplateTable displays template data in a formatted table
func (ui *InitUI) DisplayTemplateTable(header []string, rows [][]string) {
	// Get terminal width
	width, _, err := term.GetSize(uintptr(os.Stdout.Fd()))
	if err != nil {
		width = 80 // fallback width
	}

	// Calculate table width (leave some margin)
	tableWidth := width - 20

	// Convert rows to table.Row format
	var tableRows []table.Row
	for _, row := range rows {
		tableRows = append(tableRows, table.Row(row))
	}

	// Calculate column widths based on content
	nameWidth := 20    // Minimum width for template names
	sourceWidth := 30  // Minimum width for source
	versionWidth := 15 // Minimum width for version
	descWidth := 40    // Minimum width for descriptions

	// Find the maximum content width for each column
	for _, row := range rows {
		if len(row) >= 4 {
			if len(row[0]) > nameWidth {
				nameWidth = len(row[0])
			}
			if len(row[1]) > sourceWidth {
				sourceWidth = len(row[1])
			}
			if len(row[2]) > versionWidth {
				versionWidth = len(row[2])
			}
			if len(row[3]) > descWidth {
				descWidth = len(row[3])
			}
		}
	}

	// Add some padding to each column
	nameWidth += 2
	sourceWidth += 2
	versionWidth += 2
	descWidth += 2

	// Calculate total table width needed
	totalContentWidth := nameWidth + sourceWidth + versionWidth + descWidth + 8 // +8 for borders and spacing

	// If content is wider than screen, use content width; otherwise use screen width
	if totalContentWidth > tableWidth {
		tableWidth = totalContentWidth
	}

	// Create table
	t := table.New(
		table.WithColumns([]table.Column{
			{Title: "Template", Width: nameWidth},
			{Title: "Source", Width: sourceWidth},
			{Title: "Version", Width: versionWidth},
			{Title: "Description", Width: descWidth},
		}),
		table.WithRows(tableRows),
		table.WithWidth(tableWidth),
		table.WithFocused(false),
		table.WithHeight(len(tableRows)+1), // Set explicit height to minimize spacing
	)

	// Style the table with colors
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5F5FD7")). // Purple
		BorderBottom(true).
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")) // White
	s.Cell = s.Cell.
		Foreground(lipgloss.Color("#FFFFFF")). // White
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#FFFFFF")). // White
		Background(lipgloss.Color("#000000"))  // Black

	t.SetStyles(s)

	// Print the table
	fmt.Println()
	fmt.Println("Available Scaffold Templates")
	fmt.Println()
	fmt.Println(t.View())
	fmt.Println()
}
