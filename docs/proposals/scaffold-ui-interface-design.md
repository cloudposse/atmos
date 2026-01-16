# Scaffold UI Interface Design

## Overview

The `ScaffoldUI` interface abstracts all UI operations for the scaffold command, enabling dependency injection and testability. This design separates business logic from presentation concerns.

## Interface Definition

```go
package scaffold

import (
    "io"

    "github.com/cloudposse/atmos/pkg/generator/templates"
)

// ScaffoldUI defines all UI operations needed by the scaffold command.
// This interface allows us to inject mock implementations for testing.
type ScaffoldUI interface {
    // Basic Output Methods
    Info(message string) error
    Success(message string) error
    Error(message string) error
    Warning(message string) error
    Write(message string) error
    Writef(format string, args ...interface{}) error
    Writeln(message string) error

    // Interactive Prompts
    PromptForTemplate(configs map[string]templates.Configuration) (templates.Configuration, error)
    PromptForTargetDirectory(defaultDir string) (string, error)
    PromptForValue(prompt PromptConfig, defaultValue interface{}) (interface{}, error)

    // Complex Rendering
    RenderTemplateList(configs map[string]templates.Configuration) error
    RenderDryRunPreview(config *templates.Configuration, targetDir string, files []DryRunFile) error
    RenderValidationResults(results []ValidationResult) error
    RenderValidationSummary(validCount, errorCount int) error

    // File Operations Feedback
    PrintFilePath(targetDir, renderedPath string) error
    PrintFileStatus(path string, status FileStatus) error
}

// DryRunFile represents a file that would be generated.
type DryRunFile struct {
    Path        string
    Content     string
    IsTemplate  bool
    Exists      bool
    WouldCreate bool
    WouldUpdate bool
}

// FileStatus represents the status of a file operation.
type FileStatus int

const (
    FileStatusCreated FileStatus = iota
    FileStatusUpdated
    FileStatusSkipped
    FileStatusConflict
)

// ValidationResult represents the result of validating a scaffold file.
type ValidationResult struct {
    Path    string
    Valid   bool
    Errors  []string
    Message string
}
```

## Method Details

### Basic Output Methods

#### `Info(message string) error`
**Purpose**: Display informational messages to the user.

**Maps to**: `atmosui.Info(message)`

**Usage**:
```go
ui.Info("Loading scaffold templates...")
ui.Info(fmt.Sprintf("Found %d templates", len(configs)))
```

**Test Mock**:
```go
mockUI.EXPECT().Info("Loading scaffold templates...").Return(nil)
```

---

#### `Success(message string) error`
**Purpose**: Display success messages with visual feedback (green checkmark).

**Maps to**: `atmosui.Success(message)`

**Usage**:
```go
ui.Success("Scaffold generated successfully!")
ui.Success(fmt.Sprintf("Generated %d files", fileCount))
```

**Test Mock**:
```go
mockUI.EXPECT().Success(gomock.Any()).Return(nil)
```

---

#### `Error(message string) error`
**Purpose**: Display error messages with visual feedback (red X).

**Maps to**: `atmosui.Error(message)`

**Usage**:
```go
ui.Error("Failed to load template configuration")
ui.Error(fmt.Sprintf("Template '%s' not found", templateName))
```

**Test Mock**:
```go
mockUI.EXPECT().Error(gomock.Any()).Return(nil)
```

---

#### `Warning(message string) error`
**Purpose**: Display warning messages with visual feedback (yellow warning icon).

**Maps to**: `atmosui.Warning(message)`

**Usage**:
```go
ui.Warning("Target directory already exists")
ui.Warning("Some template variables are undefined")
```

**Test Mock**:
```go
mockUI.EXPECT().Warning(gomock.Any()).Return(nil)
```

---

#### `Write(message string) error`
**Purpose**: Write plain text without formatting or icons.

**Maps to**: `atmosui.Write(message)`

**Usage**:
```go
ui.Write("Processing templates...")
ui.Write("\n") // Blank line
```

**Test Mock**:
```go
mockUI.EXPECT().Write(gomock.Any()).Return(nil).AnyTimes()
```

---

#### `Writef(format string, args ...interface{}) error`
**Purpose**: Write formatted text without icons.

**Maps to**: `atmosui.Writef(format, args...)`

**Usage**:
```go
ui.Writef("Template: %s\n", config.Name)
ui.Writef("Target: %s\n", targetDir)
```

**Test Mock**:
```go
mockUI.EXPECT().Writef("Template: %s\n", "test-template").Return(nil)
```

---

#### `Writeln(message string) error`
**Purpose**: Write plain text with newline.

**Maps to**: `atmosui.Writeln(message)`

**Usage**:
```go
ui.Writeln("Available templates:")
ui.Writeln("")
```

### Interactive Prompts

#### `PromptForTemplate(configs map[string]templates.Configuration) (templates.Configuration, error)`
**Purpose**: Interactively prompt user to select a scaffold template.

**Maps to**: `generatorUI.InitUI.PromptForTemplate(...)`

**Current Implementation**:
```go
// In scaffold.go:393
func selectTemplateInteractive(
	configs map[string]templates.Configuration,
	scaffoldUI *generatorUI.InitUI,
) (templates.Configuration, error) {
	selectedName, err := scaffoldUI.PromptForTemplate("scaffold", configs)
	if err != nil {
		// ...
	}
	// ...
}
```

**New Usage**:
```go
selectedConfig, err := ui.PromptForTemplate(configs)
if err != nil {
	return err
}
```

**Test Mock**:
```go
mockUI.EXPECT().
	PromptForTemplate(gomock.Any()).
	Return(templates.Configuration{Name: "test-template"}, nil)
```

---

#### `PromptForTargetDirectory(defaultDir string) (string, error)`
**Purpose**: Interactively prompt user for target directory.

**Maps to**: Custom prompt logic (currently inline)

**New Usage**:
```go
targetDir, err := ui.PromptForTargetDirectory(".")
if err != nil {
	return err
}
```

**Test Mock**:
```go
mockUI.EXPECT().
	PromptForTargetDirectory(".").
	Return("/tmp/test", nil)
```

---

#### `PromptForValue(prompt PromptConfig, defaultValue interface{}) (interface{}, error)`
**Purpose**: Prompt user for a template variable value based on prompt configuration.

**Maps to**: Template value collection logic

**Usage**:
```go
value, err := ui.PromptForValue(PromptConfig{
	Name:        "component_name",
	Description: "Name of the component",
	Type:        "input",
	Required:    true,
}, nil)
```

**Test Mock**:
```go
mockUI.EXPECT().
	PromptForValue(gomock.Any(), gomock.Any()).
	Return("vpc", nil)
```

### Complex Rendering

#### `RenderTemplateList(configs map[string]templates.Configuration) error`
**Purpose**: Render a formatted list/table of available scaffold templates.

**Maps to**: Logic in `executeScaffoldList()` (lines 595-655)

**Current Implementation**:
```go
func executeScaffoldList(cmd *cobra.Command) error {
	// ... loads templates ...

	if err := atmosui.Writeln("\nAvailable Scaffold Templates:\n"); err != nil {
		return err
	}

	// Create table data
	header := []string{"Name", "Description", "Source", "Version"}
	rows := [][]string{}

	for name, config := range configs {
		// Build rows...
	}

	// Render table
	genCtx.UI.DisplayConfigurationTable(header, rows)

	return nil
}
```

**New Usage**:
```go
// Business logic just calls the renderer
return ui.RenderTemplateList(configs)
```

**Implementation** (in ui_impl.go):
```go
func (ui *ProductionUI) RenderTemplateList(configs map[string]templates.Configuration) error {
	if err := atmosui.Writeln("\nAvailable Scaffold Templates:\n"); err != nil {
		return err
	}

	// Build table data
	header := []string{"Name", "Description", "Source", "Version"}
	rows := [][]string{}

	for name, config := range configs {
		source := "embedded"
		if config.Source != "" {
			source = config.Source
		}

		rows = append(rows, []string{
			name,
			config.Description,
			source,
			config.Version,
		})
	}

	// Render using InitUI
	ui.initUI.DisplayConfigurationTable(header, rows)
	return nil
}
```

**Test Mock**:
```go
mockUI.EXPECT().RenderTemplateList(gomock.Any()).Return(nil)
```

---

#### `RenderDryRunPreview(config *templates.Configuration, targetDir string, files []DryRunFile) error`
**Purpose**: Render a preview of files that would be generated in dry-run mode.

**Maps to**: `renderDryRunPreview()`, `renderDryRunHeader()`, `renderDryRunFileList()` (lines 456-542)

**Current Implementation**:
```go
func renderDryRunPreview(
	selectedConfig *templates.Configuration,
	targetDir string,
	templateValues map[string]interface{},
) error {
	// Header
	if err := renderDryRunHeader(selectedConfig, targetDir); err != nil {
		return err
	}

	// Load values
	mergedValues, err := loadDryRunValues(selectedConfig, templateValues)
	if err != nil {
		return err
	}

	// Render file list
	if err := renderDryRunFileList(selectedConfig, targetDir, mergedValues); err != nil {
		return err
	}

	return atmosui.Writeln("\n💡 Use --force to overwrite existing files")
}
```

**New Usage**:
```go
files := []DryRunFile{
	{
		Path:        "src/main.go",
		Content:     "package main...",
		IsTemplate:  true,
		Exists:      false,
		WouldCreate: true,
	},
	// ...
}

return ui.RenderDryRunPreview(config, targetDir, files)
```

**Implementation** (in ui_impl.go):
```go
func (ui *ProductionUI) RenderDryRunPreview(
	config *templates.Configuration,
	targetDir string,
	files []DryRunFile,
) error {
	// Header
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
	}

	// File list
	if err := atmosui.Writeln("Files that would be generated:\n"); err != nil {
		return err
	}

	for _, file := range files {
		status := "CREATE"
		icon := "+"

		if file.Exists {
			status = "UPDATE"
			icon = "~"
		}

		if err := atmosui.Writef("  %s %s %s\n", icon, status, file.Path); err != nil {
			return err
		}
	}

	return atmosui.Writeln("\n💡 Use --force to overwrite existing files")
}
```

**Test Mock**:
```go
mockUI.EXPECT().
	RenderDryRunPreview(gomock.Any(), "/tmp/test", gomock.Any()).
	Return(nil)
```

---

#### `RenderValidationResults(results []ValidationResult) error`
**Purpose**: Render validation results for scaffold files.

**Maps to**: Logic in `validateAllScaffoldFiles()` (lines 699-720)

**Current Implementation**:
```go
func validateAllScaffoldFiles(scaffoldPaths []string) (int, int, error) {
	validCount := 0
	errorCount := 0

	for _, scaffoldPath := range scaffoldPaths {
		if err := validateSingleScaffoldFile(scaffoldPath); err != nil {
			errorCount++
			if uiErr := atmosui.Error(fmt.Sprintf("✗ %s: %v", scaffoldPath, err)); uiErr != nil {
				return 0, 0, uiErr
			}
		} else {
			validCount++
			if uiErr := atmosui.Success(fmt.Sprintf("✓ %s", scaffoldPath)); uiErr != nil {
				return 0, 0, uiErr
			}
		}
	}

	return validCount, errorCount, nil
}
```

**New Usage**:
```go
results := []ValidationResult{
	{Path: "scaffold1.yaml", Valid: true},
	{Path: "scaffold2.yaml", Valid: false, Errors: []string{"missing name"}},
}

return ui.RenderValidationResults(results)
```

**Implementation**:
```go
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
```

**Test Mock**:
```go
mockUI.EXPECT().RenderValidationResults(gomock.Any()).Return(nil)
```

---

#### `RenderValidationSummary(validCount, errorCount int) error`
**Purpose**: Render a summary of validation results.

**Maps to**: `printValidationSummary()` (lines 722-746)

**Current Implementation**:
```go
func printValidationSummary(validCount int, errorCount int) error {
	if err := atmosui.Writeln(""); err != nil {
		return err
	}

	if errorCount > 0 {
		return errUtils.Build(errUtils.ErrScaffoldValidation).
			WithExplanationf("Found %d scaffold file(s) with errors", errorCount).
			WithHintf("Valid files: %d", validCount).
			WithHintf("Files with errors: %d", errorCount).
			// ...
			Err()
	}

	return atmosui.Success(fmt.Sprintf("All %d scaffold file(s) are valid!", validCount))
}
```

**New Usage**:
```go
return ui.RenderValidationSummary(validCount, errorCount)
```

**Test Mock**:
```go
mockUI.EXPECT().RenderValidationSummary(3, 0).Return(nil)
mockUI.EXPECT().RenderValidationSummary(2, 1).Return(errors.New("validation failed"))
```

### File Operations Feedback

#### `PrintFilePath(targetDir, renderedPath string) error`
**Purpose**: Print the path of a file being generated.

**Maps to**: `printFilePath()` (lines 560-567)

**Current Implementation**:
```go
func printFilePath(targetDir string, renderedPath string) error {
	fullPath := filepath.Join(targetDir, renderedPath)
	return atmosui.Writef("  %s %s\n", "•", fullPath)
}
```

**New Usage**:
```go
err := ui.PrintFilePath(targetDir, renderedPath)
```

**Test Mock**:
```go
mockUI.EXPECT().PrintFilePath("/tmp/test", "config.yaml").Return(nil)
```

---

#### `PrintFileStatus(path string, status FileStatus) error`
**Purpose**: Print the status of a file operation (created, updated, skipped, conflict).

**New Addition** - not in current code but useful for better feedback.

**Usage**:
```go
ui.PrintFileStatus("config.yaml", FileStatusCreated)
ui.PrintFileStatus("README.md", FileStatusUpdated)
ui.PrintFileStatus("main.go", FileStatusSkipped)
```

**Implementation**:
```go
func (ui *ProductionUI) PrintFileStatus(path string, status FileStatus) error {
	var icon, statusText string

	switch status {
	case FileStatusCreated:
		icon, statusText = "+", "CREATE"
	case FileStatusUpdated:
		icon, statusText = "~", "UPDATE"
	case FileStatusSkipped:
		icon, statusText = "-", "SKIP"
	case FileStatusConflict:
		icon, statusText = "!", "CONFLICT"
	}

	return atmosui.Writef("  %s %s %s\n", icon, statusText, path)
}
```

**Test Mock**:
```go
mockUI.EXPECT().PrintFileStatus("config.yaml", FileStatusCreated).Return(nil)
```

## Production Implementation

### Complete Production UI Implementation

```go
// cmd/scaffold/ui_impl.go

package scaffold

import (
	"fmt"
	"strings"

	atmosui "github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	generatorUI "github.com/cloudposse/atmos/pkg/generator/ui"
)

// ProductionUI implements ScaffoldUI using real UI components.
type ProductionUI struct {
	initUI *generatorUI.InitUI
}

// NewProductionUI creates a production UI implementation.
func NewProductionUI(initUI *generatorUI.InitUI) ScaffoldUI {
	return &ProductionUI{initUI: initUI}
}

// Basic output methods
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

// Interactive prompts
func (ui *ProductionUI) PromptForTemplate(configs map[string]templates.Configuration) (templates.Configuration, error) {
	selectedName, err := ui.initUI.PromptForTemplate("scaffold", configs)
	if err != nil {
		return templates.Configuration{}, err
	}

	config, ok := configs[selectedName]
	if !ok {
		return templates.Configuration{}, fmt.Errorf("template not found: %s", selectedName)
	}

	return config, nil
}

func (ui *ProductionUI) PromptForTargetDirectory(defaultDir string) (string, error) {
	return ui.initUI.PromptForTargetDirectory(defaultDir)
}

func (ui *ProductionUI) PromptForValue(prompt PromptConfig, defaultValue interface{}) (interface{}, error) {
	// Implementation depends on prompt type
	switch prompt.Type {
	case "input":
		return ui.initUI.PromptForInput(prompt.Description, fmt.Sprintf("%v", defaultValue))
	case "confirm":
		return ui.initUI.PromptForConfirmation(prompt.Description)
	// ... other types
	default:
		return nil, fmt.Errorf("unsupported prompt type: %s", prompt.Type)
	}
}

// Complex rendering - implementations shown above
func (ui *ProductionUI) RenderTemplateList(configs map[string]templates.Configuration) error {
	// ... implementation shown above
}

func (ui *ProductionUI) RenderDryRunPreview(
	config *templates.Configuration,
	targetDir string,
	files []DryRunFile,
) error {
	// ... implementation shown above
}

func (ui *ProductionUI) RenderValidationResults(results []ValidationResult) error {
	// ... implementation shown above
}

func (ui *ProductionUI) RenderValidationSummary(validCount, errorCount int) error {
	// ... implementation shown above
}

// File operations
func (ui *ProductionUI) PrintFilePath(targetDir, renderedPath string) error {
	fullPath := filepath.Join(targetDir, renderedPath)
	return atmosui.Writef("  %s %s\n", "•", fullPath)
}

func (ui *ProductionUI) PrintFileStatus(path string, status FileStatus) error {
	// ... implementation shown above
}
```

## Testing Strategy

### Mock Generation
```bash
# Generate mocks
cd cmd/scaffold
go generate
```

### Unit Test Example
```go
func TestScaffoldGenerator_Generate_InteractiveMode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockUI := NewMockScaffoldUI(ctrl)
	mockLoader := NewMockTemplateLoader(ctrl)
	mockExecutor := NewMockTemplateExecutor(ctrl)

	// Test data
	testConfig := templates.Configuration{
		Name:  "test-template",
		Files: []templates.File{{Path: "test.txt"}},
	}
	configs := map[string]templates.Configuration{
		"test-template": testConfig,
	}

	// Set expectations in order
	gomock.InOrder(
		// Load templates
		mockLoader.EXPECT().LoadTemplates().Return(configs, nil),
		mockLoader.EXPECT().MergeConfiguredTemplates(configs).Return(nil),

		// Interactive selection (no template name provided)
		mockUI.EXPECT().PromptForTemplate(configs).Return(testConfig, nil),

		// Generation
		mockUI.EXPECT().Info(gomock.Any()).Return(nil),
		mockExecutor.EXPECT().Generate(testConfig, gomock.Any(), false, gomock.Any()).Return(nil),
		mockUI.EXPECT().Success(gomock.Any()).Return(nil),
	)

	// Execute
	generator := NewScaffoldGenerator(mockUI, mockLoader, mockExecutor)
	err := generator.Generate(GenerateOptions{
		TemplateName: "", // Empty triggers interactive mode
		TargetDir:    "/tmp/test",
	})

	// Assert
	require.NoError(t, err)
}
```

## Benefits of This Design

1. **Complete Testability**: Every UI operation can be mocked
2. **Clear Contracts**: Each method has a single, well-defined purpose
3. **Easy to Extend**: New UI operations just add methods to interface
4. **Type Safety**: Compile-time checking of UI calls
5. **Documentation**: Interface serves as living documentation
6. **Flexibility**: Easy to create alternative implementations (e.g., JSON output, silent mode)

## Migration Path

1. **Phase 1**: Define interface and production implementation
2. **Phase 2**: Update one command at a time (`generate` → `list` → `validate`)
3. **Phase 3**: Add tests for each migrated command
4. **Phase 4**: Remove old inline UI code once all tests pass
5. **Phase 5**: Document patterns for future commands

This phased approach ensures we can test each piece independently and roll back if needed.
