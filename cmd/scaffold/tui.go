package scaffold

import (
	"fmt"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	generatorUI "github.com/cloudposse/atmos/pkg/generator/ui"
	atmosui "github.com/cloudposse/atmos/pkg/ui"
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
	return atmosui.Info(message)
}

func (ui *ProductionUI) Success(message string) error {
	return atmosui.Success(message)
}

func (ui *ProductionUI) Error(message string) error {
	return atmosui.Error(message)
}

func (ui *ProductionUI) Warning(message string) error {
	return atmosui.Warning(message)
}

func (ui *ProductionUI) Write(message string) error {
	return atmosui.Write(message)
}

func (ui *ProductionUI) Writef(format string, args ...interface{}) error {
	return atmosui.Writef(format, args...)
}

func (ui *ProductionUI) Writeln(message string) error {
	return atmosui.Writeln(message)
}

// Interactive prompts.
func (ui *ProductionUI) PromptForTemplate(configs map[string]templates.Configuration) (templates.Configuration, error) {
	selectedName, err := ui.initUI.PromptForTemplate("scaffold", configs)
	if err != nil {
		return templates.Configuration{}, errUtils.Build(errUtils.ErrTTYRequired).
			WithExplanation("Interactive prompt requires a TTY (terminal)").
			WithHint("Use non-interactive mode: atmos scaffold generate <template> <target>").
			WithHint("Or remove the `--dry-run` flag to use interactive mode").
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

func (ui *ProductionUI) PromptForValue(prompt PromptConfig, defaultValue interface{}) (interface{}, error) {
	// Implementation depends on prompt type.
	switch prompt.Type {
	case "input":
		defStr := ""
		if defaultValue != nil {
			defStr = fmt.Sprintf("%v", defaultValue)
		}
		// Implement simple input prompt inline since InitUI doesn't have PromptForInput.
		result := defStr
		// For now, return the default value.
		// In a full implementation, this would use huh.NewInput() to prompt the user.
		return result, nil
	case "confirm":
		// Implement simple confirmation prompt inline since InitUI doesn't have PromptForConfirmation.
		// For now, return false as default.
		// In a full implementation, this would use huh.NewConfirm() to prompt the user.
		return false, nil
	default:
		return nil, errUtils.Build(errUtils.ErrScaffoldInvalidPrompt).
			WithExplanationf("Unsupported prompt type: %s", prompt.Type).
			WithHint("Valid types: input, confirm").
			WithContext("type", prompt.Type).
			Err()
	}
}

// Complex rendering.
func (ui *ProductionUI) RenderTemplateList(configs map[string]templates.Configuration) error {
	if err := atmosui.Writeln("\nAvailable Scaffold Templates:\n"); err != nil {
		return err
	}

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
	if err := atmosui.Writef("%-30s %s\n", header[0], header[1]); err != nil {
		return err
	}
	if err := atmosui.Writeln(strings.Repeat("-", 80)); err != nil {
		return err
	}

	// Print rows.
	for _, row := range rows {
		if err := atmosui.Writef("%-30s %s\n", row[0], row[1]); err != nil {
			return err
		}
	}

	return atmosui.Writeln("")
}

func (ui *ProductionUI) RenderDryRunPreview(
	config *templates.Configuration,
	targetDir string,
	files []DryRunFile,
) error {
	// Header.
	if err := atmosui.Writeln("\n🔍 Dry-run mode: Preview of files that would be generated\n"); err != nil {
		return err
	}

	if err := atmosui.Writef("Template: %s\n", config.Name); err != nil {
		return err
	}

	if targetDir != "" {
		if err := atmosui.Writef("Target: %s\n\n", targetDir); err != nil {
			return err
		}
	} else {
		if err := atmosui.Writeln(""); err != nil {
			return err
		}
	}

	// File list.
	if err := atmosui.Writeln("Files that would be generated:\n"); err != nil {
		return err
	}

	for _, file := range files {
		status := FileStatusCreated
		if file.Exists {
			status = FileStatusUpdated
		}

		if err := atmosui.Writef("  %s %s %s\n", status.Icon(), status.String(), file.Path); err != nil {
			return err
		}
	}

	return atmosui.Writeln("\n💡 Use --force to overwrite existing files")
}

func (ui *ProductionUI) RenderValidationResults(results []ValidationResult) error {
	for _, result := range results {
		if result.Valid {
			if err := atmosui.Success(fmt.Sprintf("✓ %s", result.Path)); err != nil {
				return err
			}
		} else {
			errMsg := strings.Join(result.Errors, ", ")
			if err := atmosui.Error(fmt.Sprintf("✗ %s: %s", result.Path, errMsg)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ui *ProductionUI) RenderValidationSummary(validCount, errorCount int) error {
	if err := atmosui.Writeln(""); err != nil {
		return err
	}

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

	return atmosui.Success(fmt.Sprintf("All %d scaffold file(s) are valid!", validCount))
}

// File operations feedback.
func (ui *ProductionUI) PrintFilePath(targetDir, renderedPath string) error {
	fullPath := filepath.Join(targetDir, renderedPath)
	return atmosui.Writef("  %s %s\n", "•", fullPath)
}

func (ui *ProductionUI) PrintFileStatus(path string, status FileStatus) error {
	return atmosui.Writef("  %s %s %s\n", status.Icon(), status.String(), path)
}
