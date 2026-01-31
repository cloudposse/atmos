package scaffold

import (
	"github.com/cloudposse/atmos/pkg/generator/templates"
)

// ScaffoldUI defines all UI operations needed by the scaffold command.
// This interface allows us to inject mock implementations for testing.
type ScaffoldUI interface {
	// Basic output methods - for simple messages.
	Info(message string) error
	Success(message string) error
	Error(message string) error
	Warning(message string) error
	Write(message string) error
	Writef(format string, args ...interface{}) error
	Writeln(message string) error

	// Interactive prompts - for collecting user input.
	PromptForTemplate(configs map[string]templates.Configuration) (templates.Configuration, error)
	PromptForTargetDirectory(defaultDir string) (string, error)
	PromptForValue(prompt *PromptConfig, defaultValue interface{}) (interface{}, error)

	// Complex rendering - for structured output.
	RenderTemplateList(configs map[string]templates.Configuration) error
	RenderDryRunPreview(config *templates.Configuration, targetDir string, files []DryRunFile) error
	RenderValidationResults(results []ValidationResult) error
	RenderValidationSummary(validCount, errorCount int) error

	// File operations feedback - for progress reporting.
	PrintFilePath(targetDir, renderedPath string) error
	PrintFileStatus(path string, status FileStatus) error
}

// TemplateLoader defines the interface for loading scaffold templates.
type TemplateLoader interface {
	// LoadTemplates loads all available scaffold templates from embedded sources.
	LoadTemplates() (map[string]templates.Configuration, error)

	// MergeConfiguredTemplates merges templates from atmos.yaml into the configs map.
	// It also updates the origins map to track which templates came from atmos.yaml.
	MergeConfiguredTemplates(configs map[string]templates.Configuration, origins map[string]string) error
}

// TemplateExecutor defines the interface for executing template generation.
type TemplateExecutor interface {
	// Generate executes the template generation.
	Generate(config templates.Configuration, targetDir string, force bool, values map[string]interface{}) error

	// ValidateFiles validates template files.
	ValidateFiles(files []templates.File) error
}
