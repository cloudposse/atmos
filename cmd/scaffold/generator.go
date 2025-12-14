package scaffold

import (
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	cfg "github.com/cloudposse/atmos/pkg/project/config"
)

// ScaffoldGenerator contains the business logic for scaffold operations.
// It uses dependency injection to enable unit testing with mocks.
type ScaffoldGenerator struct {
	ui             ScaffoldUI
	templateLoader TemplateLoader
	executor       TemplateExecutor
}

// NewScaffoldGenerator creates a new scaffold generator with dependency injection.
func NewScaffoldGenerator(
	ui ScaffoldUI,
	templateLoader TemplateLoader,
	executor TemplateExecutor,
) *ScaffoldGenerator {
	return &ScaffoldGenerator{
		ui:             ui,
		templateLoader: templateLoader,
		executor:       executor,
	}
}

// GenerateOptions contains all options for template generation.
type GenerateOptions struct {
	TemplateName string
	TargetDir    string
	Force        bool
	DryRun       bool
	Values       map[string]interface{}
}

// Generate executes the template generation with the given options.
func (g *ScaffoldGenerator) Generate(opts GenerateOptions) error {
	// Convert to absolute path.
	absTargetDir, err := g.resolveTargetDirectory(opts.TargetDir)
	if err != nil {
		return err
	}

	// Load all available templates.
	configs, err := g.templateLoader.LoadTemplates()
	if err != nil {
		return errUtils.Build(errUtils.ErrLoadScaffoldTemplates).
			WithExplanation("Failed to load available scaffold templates").
			WithHint("Run `atmos scaffold list` to see available templates").
			WithHint("Check that embedded templates are included in the binary").
			Err()
	}

	// Merge with configured templates.
	if err := g.templateLoader.MergeConfiguredTemplates(configs); err != nil {
		return err
	}

	// Select template.
	selectedConfig, err := g.selectTemplate(opts.TemplateName, configs)
	if err != nil {
		return err
	}

	// Dry-run mode.
	if opts.DryRun {
		return g.renderDryRunPreview(&selectedConfig, absTargetDir, opts.Values)
	}

	// Execute generation.
	return g.executeGeneration(selectedConfig, absTargetDir, opts.Force, opts.Values)
}

// ListTemplates lists all available scaffold templates.
func (g *ScaffoldGenerator) ListTemplates() error {
	// Load all available templates.
	configs, err := g.templateLoader.LoadTemplates()
	if err != nil {
		return errUtils.Build(errUtils.ErrLoadScaffoldTemplates).
			WithExplanation("Failed to load available scaffold templates").
			Err()
	}

	// Merge with configured templates.
	if err := g.templateLoader.MergeConfiguredTemplates(configs); err != nil {
		return err
	}

	// Check if any templates available.
	if len(configs) == 0 {
		return g.ui.Warning("No scaffold templates configured in atmos.yaml")
	}

	// Render the list.
	return g.ui.RenderTemplateList(configs)
}

// ValidateOptions contains options for scaffold validation.
type ValidateOptions struct {
	Path string // Path to scaffold file or directory
}

// Validate validates scaffold files.
func (g *ScaffoldGenerator) Validate(opts ValidateOptions) error {
	// Determine paths to validate.
	scaffoldPaths, err := g.determineScaffoldPathsToValidate(opts.Path)
	if err != nil {
		return err
	}

	// Check if any files found.
	if len(scaffoldPaths) == 0 {
		return g.ui.Info("No scaffold files found to validate")
	}

	// Validate all files.
	results, err := g.validateAllScaffoldFiles(scaffoldPaths)
	if err != nil {
		return err
	}

	// Render results.
	if err := g.ui.RenderValidationResults(results); err != nil {
		return err
	}

	// Count valid and error files.
	validCount := 0
	errorCount := 0
	for _, result := range results {
		if result.Valid {
			validCount++
		} else {
			errorCount++
		}
	}

	// Render summary.
	return g.ui.RenderValidationSummary(validCount, errorCount)
}

// resolveTargetDirectory converts target directory to absolute path.
func (g *ScaffoldGenerator) resolveTargetDirectory(targetDir string) (string, error) {
	if targetDir == "" {
		return "", nil
	}

	absPath, err := filepath.Abs(targetDir)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrResolveTargetDirectory).
			WithExplanationf("Cannot resolve target directory path: `%s`", targetDir).
			WithHint("Ensure the path is valid").
			WithHint("Check that the parent directory exists and is accessible").
			WithContext("target_dir", targetDir).
			WithExitCode(2).
			Err()
	}
	return absPath, nil
}

// selectTemplate selects a template either by name or interactively.
func (g *ScaffoldGenerator) selectTemplate(
	templateName string,
	configs map[string]templates.Configuration,
) (templates.Configuration, error) {
	if templateName == "" {
		// Interactive selection.
		return g.ui.PromptForTemplate(configs)
	}

	// Select by name.
	return selectTemplateByName(templateName, configs)
}

// executeGeneration executes the actual template generation.
func (g *ScaffoldGenerator) executeGeneration(
	config templates.Configuration,
	targetDir string,
	force bool,
	values map[string]interface{},
) error {
	// Inform user.
	if err := g.ui.Info(fmt.Sprintf("Generating scaffold: %s", config.Name)); err != nil {
		return err
	}

	// Execute.
	if err := g.executor.Generate(config, targetDir, force, values); err != nil {
		_ = g.ui.Error(fmt.Sprintf("Generation failed: %v", err))
		return err
	}

	// Success message.
	return g.ui.Success("Scaffold generated successfully!")
}

// renderDryRunPreview renders a preview of what would be generated.
func (g *ScaffoldGenerator) renderDryRunPreview(
	config *templates.Configuration,
	targetDir string,
	values map[string]interface{},
) error {
	// Load values for preview.
	mergedValues, err := loadDryRunValues(config, values)
	if err != nil {
		return err
	}

	// Build list of files that would be generated.
	files := make([]DryRunFile, 0, len(config.Files))
	for _, file := range config.Files {
		// Skip scaffold.yaml file itself.
		if file.Path == cfg.ScaffoldConfigFileName {
			continue
		}

		renderedPath := renderFilePath(file.Path, mergedValues)

		// Check if file exists.
		exists := false
		if targetDir != "" {
			fullPath := filepath.Join(targetDir, renderedPath)
			if _, err := os.Stat(fullPath); err == nil {
				exists = true
			}
		}

		files = append(files, DryRunFile{
			Path:        renderedPath,
			Content:     file.Content,
			IsTemplate:  file.IsTemplate,
			Exists:      exists,
			WouldCreate: !exists,
			WouldUpdate: exists,
		})
	}

	// Render through UI.
	return g.ui.RenderDryRunPreview(config, targetDir, files)
}

// determineScaffoldPathsToValidate finds scaffold files to validate.
func (g *ScaffoldGenerator) determineScaffoldPathsToValidate(path string) ([]string, error) {
	if path == "" {
		path = "."
	}

	return findScaffoldFiles(path)
}

// validateAllScaffoldFiles validates multiple scaffold files.
func (g *ScaffoldGenerator) validateAllScaffoldFiles(scaffoldPaths []string) ([]ValidationResult, error) {
	results := make([]ValidationResult, 0, len(scaffoldPaths))

	for _, scaffoldPath := range scaffoldPaths {
		// validateSingleScaffoldFile expects (path, allPaths) and returns ([]string, error).
		// For validation, we just need the error, so we ignore the returned paths.
		_, err := validateSingleScaffoldFile(scaffoldPath, scaffoldPaths)
		if err != nil {
			results = append(results, ValidationResult{
				Path:   scaffoldPath,
				Valid:  false,
				Errors: []string{err.Error()},
			})
		} else {
			results = append(results, ValidationResult{
				Path:  scaffoldPath,
				Valid: true,
			})
		}
	}

	return results, nil
}
