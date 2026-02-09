package scaffold

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	generatorUI "github.com/cloudposse/atmos/pkg/generator/ui"
	atmosui "github.com/cloudposse/atmos/pkg/ui"
)

// Terminal/table width constants.
const (
	defaultTableWidth = 80
)

// ProductionUI implements ScaffoldUI using real UI components.
type ProductionUI struct {
	initUI *generatorUI.InitUI
}

// NewProductionUI creates a new production UI implementation.
func NewProductionUI(initUI *generatorUI.InitUI) ScaffoldUI {
	return &ProductionUI{initUI: initUI}
}

// Basic output methods.
func (ui *ProductionUI) Info(message string) error {
	atmosui.Info(message)
	return nil
}

func (ui *ProductionUI) Success(message string) error {
	atmosui.Success(message)
	return nil
}

func (ui *ProductionUI) Error(message string) error {
	atmosui.Error(message)
	return nil
}

func (ui *ProductionUI) Warning(message string) error {
	atmosui.Warning(message)
	return nil
}

func (ui *ProductionUI) Write(message string) error {
	atmosui.Write(message)
	return nil
}

func (ui *ProductionUI) Writef(format string, args ...interface{}) error {
	atmosui.Writef(format, args...)
	return nil
}

func (ui *ProductionUI) Writeln(message string) error {
	atmosui.Writeln(message)
	return nil
}

// Interactive prompts.
func (ui *ProductionUI) PromptForTemplate(configs map[string]templates.Configuration) (templates.Configuration, error) {
	selectedName, err := ui.initUI.PromptForTemplate("scaffold", configs)
	if err != nil {
		// Check if user cancelled the selection.
		if errors.Is(err, huh.ErrUserAborted) {
			return templates.Configuration{}, errUtils.Build(errUtils.ErrPromptFailed).
				WithCause(err).
				WithExplanation("User cancelled template selection").
				WithContext("mode", "interactive").
				Err()
		}
		// For other errors, assume TTY issue.
		return templates.Configuration{}, errUtils.Build(errUtils.ErrTTYRequired).
			WithCause(err).
			WithExplanation("Interactive prompt requires a TTY (terminal)").
			WithHint("Use non-interactive mode: atmos scaffold generate <template> <target>").
			WithContext("mode", "interactive").
			Err()
	}

	config, ok := configs[selectedName]
	if !ok {
		return templates.Configuration{}, errUtils.Build(errUtils.ErrScaffoldNotFound).
			WithExplanationf("Template not found after selection: '%s'", selectedName).
			WithContext("template", selectedName).
			Err()
	}

	return config, nil
}

func (ui *ProductionUI) PromptForTargetDirectory(defaultDir string) (string, error) {
	// PromptForTargetDirectory in InitUI expects (templateInfo, mergedValues).
	// For scaffold, we pass nil for both since we just want a simple directory prompt.
	return ui.initUI.PromptForTargetDirectory(nil, map[string]interface{}{"default_dir": defaultDir})
}

func (ui *ProductionUI) PromptForValue(prompt *PromptConfig, defaultValue interface{}) (interface{}, error) {
	// Implementation depends on prompt type.
	switch prompt.Type {
	case "input":
		return ui.promptForInput(prompt, defaultValue)
	case "confirm":
		return ui.promptForConfirm(prompt, defaultValue)
	case "select":
		return ui.promptForSelect(prompt, defaultValue)
	case "multiselect":
		return ui.promptForMultiselect(prompt, defaultValue)
	default:
		return nil, errUtils.Build(errUtils.ErrScaffoldInvalidPrompt).
			WithExplanationf("Unsupported prompt type: %s", prompt.Type).
			WithHint("Valid types: input, select, confirm, multiselect").
			WithContext("type", prompt.Type).
			Err()
	}
}

// promptForInput prompts the user for text input using huh.
func (ui *ProductionUI) promptForInput(prompt *PromptConfig, defaultValue interface{}) (interface{}, error) {
	defStr := ""
	if defaultValue != nil {
		defStr = fmt.Sprintf("%v", defaultValue)
	}

	var result string
	title := prompt.Name
	if prompt.Description != "" {
		title = prompt.Description
	}

	input := huh.NewInput().
		Title(title).
		Value(&result)

	if defStr != "" {
		input = input.Placeholder(defStr)
		result = defStr
	}

	form := huh.NewForm(huh.NewGroup(input))
	if err := form.Run(); err != nil {
		return nil, errUtils.Build(errUtils.ErrPromptFailed).
			WithCause(err).
			WithExplanation("Failed to prompt for input").
			WithHint("Interactive prompts require a TTY (terminal)").
			WithContext("prompt", prompt.Name).
			Err()
	}

	return result, nil
}

// promptForConfirm prompts the user for confirmation using huh.
func (ui *ProductionUI) promptForConfirm(prompt *PromptConfig, defaultValue interface{}) (interface{}, error) {
	defBool := false
	if defaultValue != nil {
		if b, ok := defaultValue.(bool); ok {
			defBool = b
		}
	}

	var result bool
	title := prompt.Name
	if prompt.Description != "" {
		title = prompt.Description
	}

	confirm := huh.NewConfirm().
		Title(title).
		Value(&result).
		Affirmative("Yes").
		Negative("No")

	result = defBool

	form := huh.NewForm(huh.NewGroup(confirm))
	if err := form.Run(); err != nil {
		return nil, errUtils.Build(errUtils.ErrPromptFailed).
			WithCause(err).
			WithExplanation("Failed to prompt for confirmation").
			WithHint("Interactive prompts require a TTY (terminal)").
			WithContext("prompt", prompt.Name).
			Err()
	}

	return result, nil
}

// promptForSelect prompts the user to select one option using huh.
func (ui *ProductionUI) promptForSelect(prompt *PromptConfig, defaultValue interface{}) (interface{}, error) {
	options := ui.extractOptions(prompt.Options)
	if len(options) == 0 {
		return nil, errUtils.Build(errUtils.ErrScaffoldInvalidPrompt).
			WithExplanation("Select prompt requires options").
			WithHint("Add options to the prompt configuration").
			WithContext("prompt", prompt.Name).
			Err()
	}

	var result string
	title := prompt.Name
	if prompt.Description != "" {
		title = prompt.Description
	}

	// Set default value if provided.
	if defaultValue != nil {
		result = fmt.Sprintf("%v", defaultValue)
	}

	selectField := huh.NewSelect[string]().
		Title(title).
		Options(huh.NewOptions(options...)...).
		Value(&result)

	form := huh.NewForm(huh.NewGroup(selectField))
	if err := form.Run(); err != nil {
		return nil, errUtils.Build(errUtils.ErrPromptFailed).
			WithCause(err).
			WithExplanation("Failed to prompt for selection").
			WithHint("Interactive prompts require a TTY (terminal)").
			WithContext("prompt", prompt.Name).
			Err()
	}

	return result, nil
}

// promptForMultiselect prompts the user to select multiple options using huh.
func (ui *ProductionUI) promptForMultiselect(prompt *PromptConfig, defaultValue interface{}) (interface{}, error) {
	options := ui.extractOptions(prompt.Options)
	if len(options) == 0 {
		return nil, errUtils.Build(errUtils.ErrScaffoldInvalidPrompt).
			WithExplanation("Multiselect prompt requires options").
			WithHint("Add options to the prompt configuration").
			WithContext("prompt", prompt.Name).
			Err()
	}

	var result []string
	title := prompt.Name
	if prompt.Description != "" {
		title = prompt.Description
	}

	// Set default values if provided as a slice.
	if defaultValue != nil {
		if defaults, ok := defaultValue.([]interface{}); ok {
			for _, d := range defaults {
				result = append(result, fmt.Sprintf("%v", d))
			}
		} else if defaults, ok := defaultValue.([]string); ok {
			result = defaults
		}
	}

	multiselect := huh.NewMultiSelect[string]().
		Title(title).
		Options(huh.NewOptions(options...)...).
		Value(&result)

	form := huh.NewForm(huh.NewGroup(multiselect))
	if err := form.Run(); err != nil {
		return nil, errUtils.Build(errUtils.ErrPromptFailed).
			WithCause(err).
			WithExplanation("Failed to prompt for multi-selection").
			WithHint("Interactive prompts require a TTY (terminal)").
			WithContext("prompt", prompt.Name).
			Err()
	}

	return result, nil
}

// extractOptions converts prompt options to string slice.
// Options can be strings or objects with value/label fields.
func (ui *ProductionUI) extractOptions(rawOptions []interface{}) []string {
	var options []string
	for _, opt := range rawOptions {
		switch v := opt.(type) {
		case string:
			options = append(options, v)
		case map[string]interface{}:
			// Handle object format: {value: "x", label: "X"}
			if val, ok := v["value"].(string); ok {
				options = append(options, val)
			}
		}
	}
	return options
}

// Complex rendering.
func (ui *ProductionUI) RenderTemplateList(configs map[string]templates.Configuration) error {
	atmosui.Writeln("\nAvailable Scaffold Templates:\n")

	// Build table data.
	header := []string{"Name", "Description"}
	rows := [][]string{}

	for name, cfg := range configs {
		description := cfg.Description
		if description == "" {
			description = "-"
		}

		rows = append(rows, []string{
			name,
			description,
		})
	}

	// Simple table rendering since DisplayConfigurationTable is unexported.
	// Print header.
	atmosui.Writef("%-30s %s\n", header[0], header[1])
	atmosui.Writeln(strings.Repeat("-", defaultTableWidth))

	// Print rows.
	for _, row := range rows {
		atmosui.Writef("%-30s %s\n", row[0], row[1])
	}

	atmosui.Writeln("")
	return nil
}

func (ui *ProductionUI) RenderDryRunPreview(
	config *templates.Configuration,
	targetDir string,
	files []DryRunFile,
) error {
	// Header.
	atmosui.Writeln("\n🔍 Dry-run mode: Preview of files that would be generated\n")
	atmosui.Writef("Template: %s\n", config.Name)

	if targetDir != "" {
		atmosui.Writef("Target: %s\n\n", targetDir)
	} else {
		atmosui.Writeln("")
	}

	// File list.
	atmosui.Writeln("Files that would be generated:\n")

	for _, file := range files {
		status := FileStatusCreated
		if file.Exists {
			status = FileStatusUpdated
		}

		atmosui.Writef("  %s %s %s\n", status.Icon(), status.String(), file.Path)
	}

	atmosui.Writeln("")
	atmosui.Hint("Use --force to overwrite existing files")
	return nil
}

func (ui *ProductionUI) RenderValidationResults(results []ValidationResult) error {
	for _, result := range results {
		if result.Valid {
			atmosui.Success(fmt.Sprintf("✓ %s", result.Path))
		} else {
			errMsg := strings.Join(result.Errors, ", ")
			atmosui.Error(fmt.Sprintf("✗ %s: %s", result.Path, errMsg))
		}
	}
	return nil
}

func (ui *ProductionUI) RenderValidationSummary(validCount, errorCount int) error {
	atmosui.Writeln("")

	if errorCount > 0 {
		return errUtils.Build(errUtils.ErrScaffoldValidation).
			WithExplanationf("Found %d scaffold file(s) with errors", errorCount).
			WithHintf("Valid files: %d", validCount).
			WithHintf("Files with errors: %d", errorCount).
			WithHint("Fix the errors and run validation again").
			WithHint("Check the `scaffold` section syntax in the files").
			WithHint("Verify prompt types are valid: input, select, confirm, multiselect").
			WithContext("valid_count", fmt.Sprintf("%d", validCount)).
			WithContext("error_count", fmt.Sprintf("%d", errorCount)).
			WithExitCode(1).
			Err()
	}

	atmosui.Success(fmt.Sprintf("All %d scaffold file(s) are valid!", validCount))
	return nil
}

// File operations feedback.
func (ui *ProductionUI) PrintFilePath(targetDir, renderedPath string) error {
	fullPath := filepath.Join(targetDir, renderedPath)
	atmosui.Writef("  %s %s\n", "•", fullPath)
	return nil
}

func (ui *ProductionUI) PrintFileStatus(path string, status FileStatus) error {
	atmosui.Writef("  %s %s %s\n", status.Icon(), status.String(), path)
	return nil
}
