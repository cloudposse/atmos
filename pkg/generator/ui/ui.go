package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/generator/filesystem"
	tmpl "github.com/cloudposse/atmos/pkg/generator/templates"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/project/config"
	"github.com/cloudposse/atmos/pkg/terminal"
	atmosui "github.com/cloudposse/atmos/pkg/ui"
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
	processor    *engine.Processor
	ioCtx        iolib.Context
	term         terminal.Terminal
}

// NewInitUI creates a new InitUI instance
func NewInitUI(ioCtx iolib.Context, term terminal.Terminal) *InitUI {
	return &InitUI{
		checkmark:    "✓",
		xMark:        "✗",
		grayStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		successStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		errorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		output:       strings.Builder{},
		processor:    engine.NewProcessor(),
		ioCtx:        ioCtx,
		term:         term,
	}
}

// SetThreshold sets the threshold for merge operations
func (ui *InitUI) SetThreshold(thresholdPercent int) {
	ui.processor.SetMaxChanges(thresholdPercent)
}

// GetTerminalWidth returns the current terminal width with a fallback
func (ui *InitUI) GetTerminalWidth() int {
	width := ui.term.Width(terminal.Stdout)
	if width == 0 {
		return 80 // fallback width
	}
	return width
}

// writeOutput writes to the output buffer instead of using fmt.Printf
func (ui *InitUI) writeOutput(format string, args ...interface{}) {
	ui.output.WriteString(fmt.Sprintf(format, args...))
}

// colorSource returns a colored string for the given source value
func (ui *InitUI) colorSource(source string) string {
	switch source {
	case "scaffold":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#0000FF")).Render("scaffold") // Blue
	case "flag":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("flag") // Red
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")).Render("default") // Grey
	}
}

// flushOutput writes the accumulated output to stderr (UI channel) and clears the buffer.
// The buffered content is UI messages (configuration summaries, progress updates, etc.)
func (ui *InitUI) flushOutput() {
	if err := atmosui.Write(ui.output.String()); err != nil {
		log.Trace("Failed to flush UI output", "error", err)
	}
	ui.output.Reset()
}

// Execute runs the initialization process with UI
func (ui *InitUI) Execute(embedsConfig tmpl.Configuration, targetPath string, force, update, useDefaults bool, cmdTemplateValues map[string]interface{}) error {
	return ui.ExecuteWithDelimiters(embedsConfig, targetPath, force, update, useDefaults, cmdTemplateValues, []string{"{{", "}}"})
}

// ExecuteWithDelimiters runs the initialization process with UI and custom delimiters
func (ui *InitUI) ExecuteWithDelimiters(embedsConfig tmpl.Configuration, targetPath string, force, update, useDefaults bool, cmdTemplateValues map[string]interface{}, delimiters []string) error {
	// Defensive validation: target directory cannot be empty
	if targetPath == "" {
		return fmt.Errorf("internal error: target directory cannot be empty - use ExecuteWithInteractiveFlow for prompting")
	}

	// Early validation: check if target directory exists and handle appropriately
	if err := filesystem.ValidateTargetDirectory(targetPath, force, update); err != nil {
		return err
	}

	ui.writeOutput("Generating %s in %s\n\n", embedsConfig.Name, targetPath)

	// Check if this configuration has a scaffold.yaml file (project schema)
	if tmpl.HasScaffoldConfig(embedsConfig.Files) {
		return ui.executeWithSetup(embedsConfig, targetPath, force, update, useDefaults, cmdTemplateValues, delimiters)
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

// ExecuteWithInteractiveFlow provides a unified flow for both init and scaffold commands
// This ensures both commands have identical behavior - the only difference is the source of templates
func (ui *InitUI) ExecuteWithInteractiveFlow(
	embedsConfig tmpl.Configuration,
	targetPath string,
	force, update, useDefaults bool,
	cmdTemplateValues map[string]interface{},
) error {
	// If no target path was provided (interactive mode), prompt for it after setup
	if targetPath == "" {
		// For templates with scaffold configuration, we need to run setup first to get proper values
		if tmpl.HasScaffoldConfig(embedsConfig.Files) {
			// Create a temporary directory for setup
			tempDir, err := os.MkdirTemp("", "atmos-setup-*")
			if err != nil {
				return fmt.Errorf("failed to create temporary directory: %w", err)
			}
			defer os.RemoveAll(tempDir)

			// Load the scaffold configuration
			var scaffoldConfigFile *tmpl.File
			for i := range embedsConfig.Files {
				if embedsConfig.Files[i].Path == config.ScaffoldConfigFileName {
					scaffoldConfigFile = &embedsConfig.Files[i]
					break
				}
			}

			if scaffoldConfigFile == nil {
				return fmt.Errorf("%s not found in configuration", config.ScaffoldConfigFileName)
			}

			// Load the scaffold configuration from content
			scaffoldConfig, err := config.LoadScaffoldConfigFromContent(scaffoldConfigFile.Content)
			if err != nil {
				return fmt.Errorf("failed to load scaffold configuration: %w", err)
			}

			// Run setup to get configuration values
			mergedValues, _, err := ui.RunSetupForm(scaffoldConfig, tempDir, useDefaults, cmdTemplateValues)
			if err != nil {
				return fmt.Errorf("failed to run setup form: %w", err)
			}

			// Now prompt for target directory with evaluated template
			var err2 error
			targetPath, err2 = ui.PromptForTargetDirectory(embedsConfig, mergedValues)
			if err2 != nil {
				return fmt.Errorf("failed to prompt for target directory: %w", err2)
			}
		} else {
			// For simple templates, prompt directly
			var err2 error
			targetPath, err2 = ui.PromptForTargetDirectory(embedsConfig, nil)
			if err2 != nil {
				return fmt.Errorf("failed to prompt for target directory: %w", err2)
			}
		}
	}

	// Now execute with the determined target path
	return ui.Execute(embedsConfig, targetPath, force, update, useDefaults, cmdTemplateValues)
}

// promptForTargetDirectoryWithValues prompts for target directory with evaluated template values
func (ui *InitUI) promptForTargetDirectoryWithValues(config tmpl.Configuration, mergedValues map[string]interface{}) (string, error) {
	// Generate suggested directory name based on template and values
	suggestedDir := ui.generateSuggestedDirectoryWithValues(config, mergedValues)
	targetPath := suggestedDir

	// Form to get target directory with smart default
	pathForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Target directory").
				Description(fmt.Sprintf("Where should the files be created? (suggested: %s)", suggestedDir)).
				Placeholder(suggestedDir).
				Value(&targetPath).
				Validate(func(s string) error {
					if s == "" {
						return nil // Empty is OK, will use suggested default
					}
					return nil
				}),
		),
	)

	err := pathForm.Run()
	if err != nil {
		return "", err
	}

	// Use suggested directory if empty
	if targetPath == "" {
		targetPath = suggestedDir
	}

	return targetPath, nil
}

// generateSuggestedDirectoryWithValues generates a suggested directory name using template values
func (ui *InitUI) generateSuggestedDirectoryWithValues(config tmpl.Configuration, mergedValues map[string]interface{}) string {
	// If we have merged values, try to use them for a better suggestion
	if mergedValues != nil {
		if name, ok := mergedValues["name"].(string); ok && name != "" {
			return "./" + name
		}
		if projectName, ok := mergedValues["project_name"].(string); ok && projectName != "" {
			return "./" + projectName
		}
	}

	// Fallback to the original logic
	return "./" + filepath.Base(config.Name)
}

// executeWithCommandValues processes files using command-line template values
func (ui *InitUI) executeWithCommandValues(embedsConfig tmpl.Configuration, targetPath string, force, update bool, cmdTemplateValues map[string]interface{}) error {
	// For now, use the existing processFile method but this should be refactored
	// to use the templating processor properly
	var successCount, errorCount int
	for _, file := range embedsConfig.Files {
		// Process the file using the templating processor
		// Convert tmpl.File to engine.File
		templatingFile := engine.File{
			Path:        file.Path,
			Content:     file.Content,
			IsTemplate:  file.IsTemplate,
			Permissions: file.Permissions,
		}

		err := ui.processor.ProcessFile(templatingFile, targetPath, force, update, nil, cmdTemplateValues)

		// Display result using proper UI output
		if err != nil {
			// Check if this is a FileSkippedError
			if skipErr, ok := err.(*engine.FileSkippedError); ok {
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
func (ui *InitUI) executeWithUserConfig(embedsConfig tmpl.Configuration, targetPath string, force, update bool, userConfig *config.Config) error {
	// For now, use the existing processFileWithConfig method but this should be refactored
	// to use the templating processor properly
	var successCount, errorCount int
	for _, file := range embedsConfig.Files {
		// Process the file with user configuration using templating processor
		// Convert tmpl.File to engine.File
		templatingFile := engine.File{
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

	// Debug: Log valueSources map.
	log.Debug("valueSources map", "valueSources", valueSources)

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

	// Debug: Log valueSources to verify configuration sources.
	log.Debug("Configuration value sources", "valueSources", valueSources)

	ui.displayConfigurationTable(header, rows)

	// Flush the configuration summary before processing files
	ui.flushOutput()

	return mergedValues, valueSources, nil
}

// executeWithSetup handles any scaffold configuration with interactive prompts
func (ui *InitUI) executeWithSetup(embedsConfig tmpl.Configuration, targetPath string, force, update, useDefaults bool, cmdTemplateValues map[string]interface{}, delimiters []string) error {
	// Find the scaffold.yaml file in the configuration
	var scaffoldConfigFile *tmpl.File
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
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
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

		// Use the delimiters passed in, or get from scaffold config as fallback
		if len(delimiters) == 0 {
			delimiters = []string{"{{", "}}"}
			if scaffoldConfig != nil {
				// scaffoldConfig is of type *config.ScaffoldConfig, access Delimiters field directly
				if len(scaffoldConfig.Delimiters) == 2 {
					delimiters = scaffoldConfig.Delimiters
				}
			}
		}

		// Process the file path as a template first to check if it should be skipped
		renderedPath, pathErr := ui.processor.ProcessTemplateWithDelimiters(file.Path, targetPath, scaffoldConfig, mergedValues, delimiters)
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
		// Convert tmpl.File to engine.File
		templatingFile := engine.File{
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
		// Use the delimiters passed in, or get from scaffold config as fallback
		if len(delimiters) == 0 {
			delimiters = []string{"{{", "}}"}
			if scaffoldConfig != nil {
				// scaffoldConfig is of type *config.ScaffoldConfig, access Delimiters field directly
				if len(scaffoldConfig.Delimiters) == 2 {
					delimiters = scaffoldConfig.Delimiters
				}
			}
		}

		// Process README template with rich configuration
		processedContent, err := ui.processor.ProcessTemplateWithDelimiters(embedsConfig.README, targetPath, scaffoldConfig, mergedValues, delimiters)
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
	if err := atmosui.Writeln(""); err != nil {
		return err
	}
	if err := atmosui.Writeln(rendered); err != nil {
		return err
	}

	return nil
}

// renderREADME renders the README content using glamour
func (ui *InitUI) renderREADME(readmeContent string, targetPath string) error {
	// Process README template with default delimiters
	processedContent, err := ui.processor.ProcessTemplateWithDelimiters(readmeContent, targetPath, nil, nil, []string{"{{", "}}"})
	if err != nil {
		return fmt.Errorf("failed to process README template: %w", err)
	}

	// Render the processed content as markdown
	return ui.renderMarkdown(processedContent)
}

// displayConfigurationTable displays configuration data in a formatted table
func (ui *InitUI) displayConfigurationTable(header []string, rows [][]string) {
	// Don't display table if there are no rows to show
	if len(rows) == 0 {
		return
	}
	// Get terminal width
	width := ui.term.Width(terminal.Stdout)
	if width == 0 {
		width = 80 // fallback width
	}

	// Calculate table width (leave some margin)
	tableWidth := width - 20

	// Convert rows to table.Row format and apply source colors
	var tableRows []table.Row
	for _, row := range rows {
		// Apply color to source column based on the source value
		source := row[2]
		coloredSource := ui.colorSource(source)

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
		// For source column, we need to account for the colored strings
		source := row[2]
		coloredSource := ui.colorSource(source)
		// Use the length of the colored string (including ANSI codes) for width calculation
		if len(coloredSource) > sourceWidth {
			sourceWidth = len(coloredSource)
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
	width := ui.term.Width(terminal.Stdout)
	if width == 0 {
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

	// Write the table to UI channel.
	if err := atmosui.Writeln(""); err != nil {
		log.Trace("Failed to write blank line", "error", err)
	}
	if err := atmosui.Writeln("Available Scaffold Templates"); err != nil {
		log.Trace("Failed to write table header", "error", err)
	}
	if err := atmosui.Writeln(""); err != nil {
		log.Trace("Failed to write blank line", "error", err)
	}
	if err := atmosui.Writeln(t.View()); err != nil {
		log.Trace("Failed to write table", "error", err)
	}
	if err := atmosui.Writeln(""); err != nil {
		log.Trace("Failed to write blank line", "error", err)
	}
}

// PromptForTemplate prompts the user to select a template from available options
// This works for both init (embeds) and scaffold (local/remote) templates
func (ui *InitUI) PromptForTemplate(templateType string, templates interface{}) (string, error) {
	var options []huh.Option[string]
	var templateNames []string

	switch templateType {
	case "embeds":
		// Handle tmpl.Configuration map
		if configs, ok := templates.(map[string]tmpl.Configuration); ok {
			// Build config keys for consistent ordering
			for key := range configs {
				templateNames = append(templateNames, key)
			}
			sort.Strings(templateNames)

			for _, key := range templateNames {
				config := configs[key]
				displayText := fmt.Sprintf("%-15s   %-35s   %s", key, config.Name, config.Description)
				options = append(options, huh.NewOption(displayText, key))
			}
		}

	case "scaffold":
		// Handle scaffold templates from atmos.yaml
		if templatesMap, ok := templates.(map[string]interface{}); ok {
			for templateName, templateConfig := range templatesMap {
				templateMap, ok := templateConfig.(map[string]interface{})
				if !ok {
					continue
				}

				description := ""
				if desc, ok := templateMap["description"].(string); ok {
					description = desc
				}

				source := ""
				if src, ok := templateMap["source"].(string); ok {
					source = src
				}

				displayText := fmt.Sprintf("%-20s   %s", templateName, description)
				if source != "" {
					displayText += fmt.Sprintf(" (from %s)", source)
				}

				options = append(options, huh.NewOption(displayText, templateName))
				templateNames = append(templateNames, templateName)
			}
		}
	}

	if len(options) == 0 {
		return "", fmt.Errorf("no templates available")
	}

	var selectedTemplate string

	// Create the selection form
	selectionForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Select a %s template", templateType)).
				Description(fmt.Sprintf("Choose from the available %s templates (press 'q' to quit)", templateType)).
				Options(options...).
				Value(&selectedTemplate),
		),
	)

	err := selectionForm.Run()
	if err != nil {
		return "", err
	}

	// Display selected template details
	if err := atmosui.Writeln(""); err != nil {
		log.Trace("Failed to write blank line", "error", err)
	}
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Padding(0, 1)

	if err := atmosui.Writeln(descStyle.Render(fmt.Sprintf("Selected template: %s", selectedTemplate))); err != nil {
		log.Trace("Failed to write template selection", "error", err)
	}
	if err := atmosui.Writeln(""); err != nil {
		log.Trace("Failed to write blank line", "error", err)
	}

	return selectedTemplate, nil
}

// PromptForTargetDirectory prompts the user for the target directory with evaluated template values
// This works for both init and scaffold commands
func (ui *InitUI) PromptForTargetDirectory(templateInfo interface{}, mergedValues map[string]interface{}) (string, error) {
	// Generate suggested directory name based on template and values
	suggestedDir := ui.generateSuggestedDirectoryWithTemplateInfo(templateInfo, mergedValues)
	targetPath := suggestedDir

	// Form to get target directory with smart default
	pathForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Target directory").
				Description(fmt.Sprintf("Where should the files be created? (suggested: %s)", suggestedDir)).
				Placeholder(suggestedDir).
				Value(&targetPath).
				Validate(func(s string) error {
					if s == "" {
						return nil // Empty is OK, will use suggested default
					}
					return nil
				}),
		),
	)

	err := pathForm.Run()
	if err != nil {
		return "", err
	}

	// Use suggested directory if empty
	if targetPath == "" {
		targetPath = suggestedDir
	}

	return targetPath, nil
}

// generateSuggestedDirectoryWithTemplateInfo generates a suggested directory name using template info and values
func (ui *InitUI) generateSuggestedDirectoryWithTemplateInfo(templateInfo interface{}, mergedValues map[string]interface{}) string {
	// If we have merged values, try to use them for a better suggestion
	if mergedValues != nil {
		if name, ok := mergedValues["name"].(string); ok && name != "" {
			return "./" + name
		}
		if projectName, ok := mergedValues["project_name"].(string); ok && projectName != "" {
			return "./" + projectName
		}
	}

	// Try to extract name from template info
	switch info := templateInfo.(type) {
	case tmpl.Configuration:
		return "./" + filepath.Base(info.Name)
	case map[string]interface{}:
		if name, ok := info["name"].(string); ok && name != "" {
			return "./" + name
		}
	}

	// Fallback
	return "./new-project"
}

// DisplayScaffoldTemplateTable displays scaffold templates in a table format
func (ui *InitUI) DisplayScaffoldTemplateTable(templatesMap map[string]interface{}) {
	// Extract template data for table display
	var rows [][]string
	for templateName, templateConfig := range templatesMap {
		templateMap, ok := templateConfig.(map[string]interface{})
		if !ok {
			continue
		}

		// Get template source
		source, _ := templateMap["source"].(string)
		if source == "" {
			source = "unknown"
		}

		// Get template description (if available)
		description := ""
		if desc, ok := templateMap["description"].(string); ok {
			description = desc
		}

		// Get template ref (if available)
		ref := ""
		if r, ok := templateMap["ref"].(string); ok {
			ref = r
		}

		rows = append(rows, []string{templateName, source, ref, description})
	}

	header := []string{"Template", "Source", "Version", "Description"}
	ui.DisplayTemplateTable(header, rows)
}
