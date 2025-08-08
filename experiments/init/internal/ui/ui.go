package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/hairyhenderson/gomplate/v3"
	"github.com/hairyhenderson/gomplate/v3/data"

	"github.com/cloudposse/atmos/experiments/init/embeds"
	"github.com/cloudposse/atmos/experiments/init/internal/config"
	"github.com/sergi/go-diff/diffmatchpatch"
)

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
	maxChanges   int
	output       strings.Builder
}

// NewInitUI creates a new InitUI instance
func NewInitUI() *InitUI {
	return &InitUI{
		checkmark:    "✓",
		xMark:        "✗",
		grayStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		successStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		errorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		maxChanges:   10, // Default threshold of 10 changes
		output:       strings.Builder{},
	}
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
	if err := ui.validateTargetDirectory(targetPath, force, update); err != nil {
		return err
	}

	ui.writeOutput("Initializing %s in %s\n\n", embedsConfig.Name, targetPath)

	// Check if this configuration has a project-config.yaml file (setup config)
	if ui.hasProjectConfig(embedsConfig) {
		return ui.executeWithSetup(embedsConfig, targetPath, force, update, useDefaults, cmdTemplateValues)
	}

	// Load user configuration and prompt if needed
	userConfig, err := ui.loadUserConfiguration()
	if err != nil {
		return fmt.Errorf("failed to load user configuration: %w", err)
	}

	// Process each file
	var successCount, errorCount int
	for _, file := range embedsConfig.Files {
		// Process the file with user configuration
		err := ui.processFileWithConfig(file, targetPath, force, update, userConfig)

		// Display result using proper UI output
		if err != nil {
			errorCount++
			ui.writeOutput("  %s %s %s\n",
				ui.errorStyle.Render(ui.xMark),
				file.Path,
				ui.grayStyle.Render(fmt.Sprintf("(error: %v)", err)))
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
		// Don't render README if there were errors - flush output and return error immediately
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

// processFile handles the creation of a single file
func (ui *InitUI) processFile(file embeds.File, targetPath string, force, update bool) error {
	// Create full file path
	fullPath := filepath.Join(targetPath, file.Path)

	// Create directory if needed
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Check if file already exists
	if _, err := os.Stat(fullPath); err == nil {
		// File exists, handle based on flags
		if !force && !update {
			return fmt.Errorf("file already exists: %s (use --force to overwrite or --update to merge)", file.Path)
		}

		if update {
			// Attempt 3-way merge
			if err := ui.mergeFile(fullPath, file, targetPath); err != nil {
				return fmt.Errorf("failed to merge file %s: %w", file.Path, err)
			}
			return nil
		}
		// force flag is set, continue to overwrite
	}

	// Process content if it's a template
	content := file.Content
	if file.IsTemplate {
		processedContent, err := ui.processTemplate(content, targetPath, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to process template: %w", err)
		}
		content = processedContent
	}

	// Write file
	if err := os.WriteFile(fullPath, []byte(content), file.Permissions); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// processTemplate processes Go templates in file content
func (ui *InitUI) processTemplate(content string, targetPath string, projectConfig *config.ProjectConfig, userValues map[string]interface{}) (string, error) {
	// Create template data with rich configuration
	templateData := map[string]interface{}{
		"ProjectName":        filepath.Base(targetPath),
		"ProjectDescription": "An Atmos project for managing infrastructure as code",
		"TargetPath":         targetPath,
		"Config":             userValues, // Access config values via .Config.Foobar
	}

	// Create gomplate data context
	d := data.Data{}
	ctx := context.TODO()

	// Add Gomplate, Sprig and custom template functions
	funcs := template.FuncMap{}

	// Add gomplate functions
	gomplateFuncs := gomplate.CreateFuncs(ctx, &d)
	for k, v := range gomplateFuncs {
		funcs[k] = v
	}

	// Add sprig functions
	sprigFuncs := sprig.FuncMap()
	for k, v := range sprigFuncs {
		funcs[k] = v
	}

	// Add custom functions
	funcs["config"] = func(key string) interface{} {
		return userValues[key]
	}

	// Parse and execute template with gomplate
	tmpl, err := template.New("init").Funcs(funcs).Parse(content)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var result strings.Builder
	if err := tmpl.Execute(&result, templateData); err != nil {
		// Add detailed debugging information for template execution errors
		return "", fmt.Errorf("failed to execute template: %w\nTemplate data: %+v\nTemplate content preview: %s",
			err, templateData, truncateString(content, 300))
	}

	return result.String(), nil
}

// processFileWithRichConfig processes a file with rich project configuration
func (ui *InitUI) processFileWithRichConfig(file embeds.File, targetPath string, force, update bool, projectConfig *config.ProjectConfig, userValues map[string]interface{}) error {
	// Create full file path
	fullPath := filepath.Join(targetPath, file.Path)

	// Create directory if needed
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Check if file already exists
	if _, err := os.Stat(fullPath); err == nil {
		// File exists, handle based on flags
		if !force && !update {
			return fmt.Errorf("file already exists: %s (use --force to overwrite or --update to merge)", file.Path)
		}

		if update {
			// Process template first, then attempt 3-way merge
			processedContent, err := ui.processTemplate(file.Content, targetPath, projectConfig, userValues)
			if err != nil {
				return fmt.Errorf("failed to process template for file %s: %w", file.Path, err)
			}

			// Create a temporary file with processed content for merging
			tempFile := file
			tempFile.Content = processedContent

			if err := ui.mergeFile(fullPath, tempFile, targetPath); err != nil {
				return fmt.Errorf("failed to merge file %s: %w", file.Path, err)
			}
			return nil
		}
		// force flag is set, continue to overwrite
	}

	// Process all files as templates to allow configuration values to be used
	content := file.Content

	processedContent, err := ui.processTemplate(content, targetPath, projectConfig, userValues)
	if err != nil {
		// Add detailed debugging information
		return fmt.Errorf("failed to process template for file %s: %w\nTemplate content preview: %s\nUser values: %+v",
			file.Path, err,
			truncateString(content, 200),
			userValues)
	}
	content = processedContent

	// Write file
	if err := os.WriteFile(fullPath, []byte(content), file.Permissions); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// executeWithSetup handles any project configuration with interactive prompts
func (ui *InitUI) executeWithSetup(embedsConfig embeds.Configuration, targetPath string, force, update, useDefaults bool, cmdTemplateValues map[string]interface{}) error {
	// Find the project-config.yaml file in the configuration
	var projectConfigFile *embeds.File
	for i := range embedsConfig.Files {
		if embedsConfig.Files[i].Path == "project-config.yaml" {
			projectConfigFile = &embedsConfig.Files[i]
			break
		}
	}

	if projectConfigFile == nil {
		return fmt.Errorf("project-config.yaml not found in rich-project configuration")
	}

	// Create directory if needed
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Load the project configuration from embedded content (don't write to target folder)
	projectConfig, err := config.LoadProjectConfigFromContent(projectConfigFile.Content)
	if err != nil {
		return fmt.Errorf("failed to load project configuration: %w", err)
	}

	// Load existing user values from the project directory
	userValues, err := config.LoadUserValues(targetPath)
	if err != nil {
		return fmt.Errorf("failed to load user values: %w", err)
	}

	// Deep merge project defaults with user values
	mergedValues := config.DeepMerge(projectConfig, userValues)

	// Override with command-line config values (highest priority)
	for key, value := range cmdTemplateValues {
		mergedValues[key] = value
	}

	// Prompt the user to edit the configuration values unless --use-defaults is specified
	// This allows them to review and modify values from command line, config, or defaults
	if !useDefaults {
		if err := config.PromptForProjectConfig(projectConfig, mergedValues); err != nil {
			return fmt.Errorf("failed to prompt for configuration: %w", err)
		}
	} else {
		// Show configuration summary when using defaults
		var sb strings.Builder
		fmt.Fprintf(&sb,
			"%s\n\n",
			lipgloss.NewStyle().Bold(true).Render("CONFIGURATION SUMMARY"),
		)

		// Add all configured values to summary
		for key := range projectConfig.Fields {
			if value, exists := mergedValues[key]; exists {
				switch v := value.(type) {
				case []string:
					if len(v) > 0 {
						fmt.Fprintf(&sb, "%s: %s\n",
							lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render(key),
							lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render(strings.Join(v, ", ")))
					}
				case bool:
					fmt.Fprintf(&sb, "%s: %t\n",
						lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render(key), v)
				default:
					fmt.Fprintf(&sb, "%s: %s\n",
						lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render(key),
						lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render(fmt.Sprintf("%v", v)))
				}
			}
		}

		ui.writeOutput("%s\n\n",
			lipgloss.NewStyle().
				Width(50).
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("63")).
				Padding(1, 2).
				Render(sb.String()),
		)
	}

	// Save the user values
	if err := config.SaveUserValues(targetPath, mergedValues); err != nil {
		return fmt.Errorf("failed to save user values: %w", err)
	}

	// Process each file with rich configuration
	var successCount, errorCount int
	var failedFiles []string
	for _, file := range embedsConfig.Files {
		// Skip the project-config.yaml as it's only used for schema definition
		if file.Path == "project-config.yaml" {
			continue
		}

		// Process the file with rich configuration using the updated mergedValues
		err := ui.processFileWithRichConfig(file, targetPath, force, update, projectConfig, mergedValues)

		// Display result using proper UI output
		if err != nil {
			errorCount++
			failedFiles = append(failedFiles, file.Path)
			ui.writeOutput("  %s %s %s\n",
				ui.errorStyle.Render(ui.xMark),
				file.Path,
				ui.grayStyle.Render(fmt.Sprintf("(error: %v)", err)))
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
		// Don't render README if there were errors - flush output and return error immediately
		ui.flushOutput()
		return fmt.Errorf("failed to initialize files: %s", strings.Join(failedFiles, ", "))
	} else {
		ui.writeOutput("Initialized %d files.\n", successCount)
	}

	// Flush all output before rendering README
	ui.flushOutput()

	// Only render README if all files were successful
	if embedsConfig.README != "" {
		if err := ui.renderREADMEWithRichConfig(embedsConfig.README, targetPath, projectConfig, mergedValues); err != nil {
			return err
		}
	}

	return nil
}

// renderMarkdown renders markdown content using glamour
func (ui *InitUI) renderMarkdown(markdownContent string) error {
	// Create glamour renderer
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
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
	processedContent, err := ui.processTemplate(readmeContent, targetPath, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to process README template: %w", err)
	}

	// Render the processed content as markdown
	return ui.renderMarkdown(processedContent)
}

// renderREADMEWithRichConfig renders the README content using glamour with rich configuration
func (ui *InitUI) renderREADMEWithRichConfig(readmeContent string, targetPath string, projectConfig *config.ProjectConfig, userValues map[string]interface{}) error {
	// Process README template with rich configuration
	processedContent, err := ui.processTemplate(readmeContent, targetPath, projectConfig, userValues)
	if err != nil {
		return fmt.Errorf("failed to process README template: %w", err)
	}

	// Render the processed content as markdown
	return ui.renderMarkdown(processedContent)
}

// mergeFile attempts a 3-way merge for existing files
func (ui *InitUI) mergeFile(existingPath string, file embeds.File, targetPath string) error {
	// Read existing file content
	existingContent, err := os.ReadFile(existingPath)
	if err != nil {
		return fmt.Errorf("failed to read existing file: %w", err)
	}

	// Process new content
	newContent := file.Content
	if file.IsTemplate {
		processedContent, err := ui.processTemplate(newContent, targetPath, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to process template: %w", err)
		}
		newContent = processedContent
	}

	// Perform 3-way merge
	mergedContent, err := ui.performThreeWayMerge(string(existingContent), newContent, file.Path)
	if err != nil {
		return fmt.Errorf("failed to perform 3-way merge: %w", err)
	}

	// Write merged content
	if err := os.WriteFile(existingPath, []byte(mergedContent), file.Permissions); err != nil {
		return fmt.Errorf("failed to write merged file: %w", err)
	}

	return nil
}

// performThreeWayMerge implements a proper 3-way merge algorithm using diff library
func (ui *InitUI) performThreeWayMerge(existingContent, newContent, fileName string) (string, error) {
	// Use diffmatchpatch to compute the diff between existing and new content
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(existingContent, newContent, true)

	// Check if the diff is too complex (too many changes)
	changeCount := 0
	for _, diff := range diffs {
		if diff.Type != diffmatchpatch.DiffEqual {
			changeCount++
		}
	}

	// If there are too many changes, refuse to merge
	if changeCount > ui.maxChanges {
		return "", fmt.Errorf("too many changes detected (%d changes). Use --force to overwrite or manually merge", changeCount)
	}

	// Apply the diff to create a merged result
	mergedContent := dmp.DiffText2(diffs)

	// Check for conflicts by looking for diff markers
	if strings.Contains(mergedContent, "<<<<<<<") || strings.Contains(mergedContent, "=======") || strings.Contains(mergedContent, ">>>>>>>") {
		// There are conflicts, let's handle them intelligently
		mergedContent = ui.resolveConflicts(mergedContent, fileName)
	}

	return mergedContent, nil
}

// resolveConflicts handles merge conflicts by preserving user customizations
func (ui *InitUI) resolveConflicts(content, fileName string) string {
	lines := strings.Split(content, "\n")
	var resolvedLines []string
	var inConflict bool
	var conflictBuffer []string

	for _, line := range lines {
		if strings.HasPrefix(line, "<<<<<<<") {
			inConflict = true
			conflictBuffer = []string{}
			continue
		}

		if strings.HasPrefix(line, "=======") {
			// Middle of conflict - switch to "theirs" side
			conflictBuffer = []string{}
			continue
		}

		if strings.HasPrefix(line, ">>>>>>>") {
			inConflict = false
			// Resolve the conflict by preferring existing content (user customizations)
			resolvedLines = append(resolvedLines, ui.resolveConflictBlock(conflictBuffer, fileName)...)
			continue
		}

		if inConflict {
			conflictBuffer = append(conflictBuffer, line)
		} else {
			resolvedLines = append(resolvedLines, line)
		}
	}

	return strings.Join(resolvedLines, "\n")
}

// resolveConflictBlock resolves a single conflict block
func (ui *InitUI) resolveConflictBlock(conflictLines []string, fileName string) []string {
	var resolved []string

	// Add conflict resolution marker
	resolved = append(resolved, fmt.Sprintf("# CONFLICT RESOLVED for %s", fileName))
	resolved = append(resolved, "# Preserving user customizations and adding new template content")
	resolved = append(resolved, "")

	// For now, preserve all lines from the conflict
	// In a more sophisticated implementation, you'd analyze the content
	// and make intelligent decisions about what to keep
	for _, line := range conflictLines {
		if strings.TrimSpace(line) != "" {
			resolved = append(resolved, line)
		}
	}

	resolved = append(resolved, "")
	return resolved
}

// isUserCustomization checks if a line looks like user customization
func (ui *InitUI) isUserCustomization(line string) bool {
	line = strings.TrimSpace(line)

	// Skip empty lines and comments
	if line == "" || strings.HasPrefix(line, "#") {
		return false
	}

	// Check for common user customization patterns
	customizationPatterns := []string{
		"# Custom",
		"# Modified",
		"# Added",
		"# TODO",
		"# FIXME",
		"custom_",
		"my_",
		"local_",
	}

	for _, pattern := range customizationPatterns {
		if strings.Contains(strings.ToLower(line), strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

// loadUserConfiguration loads user configuration and prompts if needed
func (ui *InitUI) loadUserConfiguration() (*config.Config, error) {
	configPath, err := config.GetConfigPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get config path: %w", err)
	}

	manager := config.NewManager(configPath)
	userConfig, err := manager.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// If we don't have user configuration, prompt for it
	if userConfig.Author == "" || userConfig.Year == "" {
		fmt.Println("Please provide some configuration details:")
		fmt.Println()

		if err := manager.PromptUser(userConfig); err != nil {
			return nil, fmt.Errorf("failed to prompt user: %w", err)
		}

		// Save the configuration for future use
		if err := manager.Save(userConfig); err != nil {
			return nil, fmt.Errorf("failed to save configuration: %w", err)
		}

		fmt.Println()
	}

	return userConfig, nil
}

// processFileWithConfig processes a file with user configuration
func (ui *InitUI) processFileWithConfig(file embeds.File, targetPath string, force, update bool, userConfig *config.Config) error {
	// Create full file path
	fullPath := filepath.Join(targetPath, file.Path)

	// Create directory if needed
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Check if file already exists
	if _, err := os.Stat(fullPath); err == nil {
		// File exists, handle based on flags
		if !force && !update {
			return fmt.Errorf("file already exists: %s (use --force to overwrite or --update to merge)", file.Path)
		}

		if update {
			// Attempt 3-way merge
			if err := ui.mergeFile(fullPath, file, targetPath); err != nil {
				return fmt.Errorf("failed to merge file %s: %w", file.Path, err)
			}
			return nil
		}
		// force flag is set, continue to overwrite
	}

	// Process content if it's a template
	content := file.Content
	if file.IsTemplate {
		processedContent, err := ui.processTemplateWithConfig(content, targetPath, userConfig)
		if err != nil {
			return fmt.Errorf("failed to process template: %w", err)
		}
		content = processedContent
	}

	// Write file
	if err := os.WriteFile(fullPath, []byte(content), file.Permissions); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// processTemplateWithConfig processes Go templates with user configuration
func (ui *InitUI) processTemplateWithConfig(content string, targetPath string, userConfig *config.Config) (string, error) {
	// Create template data with user configuration
	data := map[string]interface{}{
		"ProjectName":        filepath.Base(targetPath),
		"ProjectDescription": "An Atmos project for managing infrastructure as code",
		"TargetPath":         targetPath,
		"Author":             userConfig.Author,
		"Year":               userConfig.Year,
		"License":            userConfig.License[0], // Use first license as default
	}

	// Add template functions to avoid conflicts with reserved names
	funcMap := template.FuncMap{
		"default": func(value, defaultValue interface{}) interface{} {
			if value == nil || value == "" {
				return defaultValue
			}
			return value
		},
	}

	// Parse and execute template
	tmpl, err := template.New("init").Funcs(funcMap).Parse(content)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var result strings.Builder
	if err := tmpl.Execute(&result, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return result.String(), nil
}

// hasProjectConfig checks if a configuration contains a project-config.yaml file
func (ui *InitUI) hasProjectConfig(config embeds.Configuration) bool {
	for _, file := range config.Files {
		if file.Path == "project-config.yaml" {
			return true
		}
	}
	return false
}

// validateTargetDirectory checks if the target directory exists and validates the operation
func (ui *InitUI) validateTargetDirectory(targetPath string, force, update bool) error {
	// Check if target directory exists
	if _, err := os.Stat(targetPath); err == nil {
		// Directory exists, check if it has any files that would conflict
		entries, err := os.ReadDir(targetPath)
		if err != nil {
			return fmt.Errorf("failed to read target directory: %w", err)
		}

		// Filter out hidden files and directories
		var visibleEntries []string
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".") {
				visibleEntries = append(visibleEntries, entry.Name())
			}
		}

		if len(visibleEntries) > 0 {
			if !force && !update {
				return fmt.Errorf("target directory '%s' already exists and contains files: %s (use --force to overwrite or --update to merge)",
					targetPath, strings.Join(visibleEntries, ", "))
			}
		}
	}

	return nil
}
