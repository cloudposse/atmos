# Scaffold Command Testability Refactoring

## Problem Statement

The scaffold command (`cmd/scaffold/scaffold.go`) currently has **50.11% test coverage** with 225 missed lines. The primary blocker is that business logic is tightly coupled with UI rendering, making unit testing impossible without significant refactoring.

### Current Architecture Issues

1. **Direct UI Dependencies**: Functions call `atmosui.Info()`, `atmosui.Error()`, etc. directly
2. **No Dependency Injection**: UI components are instantiated within business logic
3. **Untestable Functions**: ~400 lines of code cannot be unit tested:
   - `executeScaffoldGenerate` (lines 255-288)
   - `executeScaffoldList` (lines 595-655)
   - `executeValidateScaffold` (lines 659-744)
   - `renderDryRunPreview`, `renderDryRunHeader`, `renderDryRunFileList`, `printFilePath`

## Solution: Interface-Driven Design

Following CLAUDE.md architectural patterns, we will:
1. **Define interfaces** for UI and template operations
2. **Use dependency injection** to pass dependencies
3. **Separate business logic** from UI rendering
4. **Generate mocks** for unit testing

## Refactoring Plan

### Phase 1: Define Core Interfaces

Create `cmd/scaffold/interfaces.go`:

```go
package scaffold

import (
	"github.com/cloudposse/atmos/pkg/generator/templates"
	generatorUI "github.com/cloudposse/atmos/pkg/generator/ui"
)

// ScaffoldUI defines the interface for UI operations during scaffold commands.
type ScaffoldUI interface {
	// Output methods
	Info(message string) error
	Success(message string) error
	Error(message string) error
	Warning(message string) error
	Write(message string) error
	Writef(format string, args ...interface{}) error

	// Interactive prompts
	PromptForTemplate(configs map[string]templates.Configuration) (templates.Configuration, error)
	PromptForTargetDirectory() (string, error)

	// Rendering
	RenderTemplateList(configs map[string]templates.Configuration) error
	RenderDryRunPreview(config *templates.Configuration, targetDir string, files []DryRunFile) error
	RenderValidationSummary(validCount, errorCount int) error
}

// TemplateLoader defines the interface for loading scaffold templates.
type TemplateLoader interface {
	LoadTemplates() (map[string]templates.Configuration, error)
	MergeConfiguredTemplates(configs map[string]templates.Configuration) error
}

// TemplateExecutor defines the interface for executing template generation.
type TemplateExecutor interface {
	Generate(config templates.Configuration, targetDir string, force bool, values map[string]interface{}) error
	ValidateFiles(files []templates.File) error
}

// DryRunFile represents a file that would be generated in dry-run mode.
type DryRunFile struct {
	Path    string
	Content string
	Exists  bool
}
```

### Phase 2: Create Production Implementations

Create `cmd/scaffold/ui_impl.go`:

```go
package scaffold

import (
	"fmt"

	atmosui "github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	generatorUI "github.com/cloudposse/atmos/pkg/generator/ui"
)

// ProductionUI implements ScaffoldUI using the real UI components.
type ProductionUI struct {
	initUI *generatorUI.InitUI
}

// NewProductionUI creates a new production UI implementation.
func NewProductionUI(initUI *generatorUI.InitUI) ScaffoldUI {
	return &ProductionUI{initUI: initUI}
}

func (ui *ProductionUI) Info(message string) error {
	return atmosui.Info(message)
}

func (ui *ProductionUI) Success(message string) error {
	return atmosui.Success(message)
}

// ... implement all ScaffoldUI methods
```

Create `cmd/scaffold/template_loader_impl.go`:

```go
package scaffold

import (
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/project/config"
)

// ProductionTemplateLoader implements TemplateLoader.
type ProductionTemplateLoader struct{}

func NewProductionTemplateLoader() TemplateLoader {
	return &ProductionTemplateLoader{}
}

func (l *ProductionTemplateLoader) LoadTemplates() (map[string]templates.Configuration, error) {
	return templates.GetAvailableConfigurations()
}

func (l *ProductionTemplateLoader) MergeConfiguredTemplates(configs map[string]templates.Configuration) error {
	return mergeConfiguredTemplates(configs)
}
```

### Phase 3: Refactor Business Logic

Create `cmd/scaffold/generator.go`:

```go
package scaffold

import (
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/generator/templates"
	errUtils "github.com/cloudposse/atmos/errors"
)

// ScaffoldGenerator contains the business logic for scaffold operations.
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
	// Convert to absolute path
	absTargetDir, err := g.resolveTargetDirectory(opts.TargetDir)
	if err != nil {
		return err
	}

	// Load all available templates
	configs, err := g.templateLoader.LoadTemplates()
	if err != nil {
		return errUtils.Build(errUtils.ErrLoadScaffoldTemplates).
			WithExplanation("Failed to load available scaffold templates").
			Err()
	}

	// Merge with configured templates
	if err := g.templateLoader.MergeConfiguredTemplates(configs); err != nil {
		return err
	}

	// Select template
	selectedConfig, err := g.selectTemplate(opts.TemplateName, configs)
	if err != nil {
		return err
	}

	// Dry-run mode
	if opts.DryRun {
		return g.renderDryRunPreview(&selectedConfig, absTargetDir, opts.Values)
	}

	// Execute generation
	return g.executeGeneration(selectedConfig, absTargetDir, opts.Force, opts.Values)
}

// resolveTargetDirectory is now a method that can be tested independently.
func (g *ScaffoldGenerator) resolveTargetDirectory(targetDir string) (string, error) {
	if targetDir == "" {
		return "", nil
	}

	absPath, err := filepath.Abs(targetDir)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrResolveTargetDirectory).
			WithExplanationf("Cannot resolve target directory path: `%s`", targetDir).
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
		// Interactive selection
		return g.ui.PromptForTemplate(configs)
	}

	// Select by name
	return selectTemplateByName(templateName, configs)
}

// executeGeneration executes the actual template generation.
func (g *ScaffoldGenerator) executeGeneration(
	config templates.Configuration,
	targetDir string,
	force bool,
	values map[string]interface{},
) error {
	// Inform user
	if err := g.ui.Info(fmt.Sprintf("Generating scaffold: %s", config.Name)); err != nil {
		return err
	}

	// Execute
	if err := g.executor.Generate(config, targetDir, force, values); err != nil {
		_ = g.ui.Error(fmt.Sprintf("Generation failed: %v", err))
		return err
	}

	// Success message
	return g.ui.Success("Scaffold generated successfully!")
}

// renderDryRunPreview renders a preview of what would be generated.
func (g *ScaffoldGenerator) renderDryRunPreview(
	config *templates.Configuration,
	targetDir string,
	values map[string]interface{},
) error {
	// Load values for preview
	mergedValues, err := loadDryRunValues(config, values)
	if err != nil {
		return err
	}

	// Build list of files that would be generated
	files := make([]DryRunFile, 0, len(config.Files))
	for _, file := range config.Files {
		if file.Path == config.ScaffoldConfigFileName {
			continue // Skip scaffold.yaml
		}

		renderedPath := renderFilePath(file.Path, mergedValues)
		files = append(files, DryRunFile{
			Path:    renderedPath,
			Content: file.Content,
			Exists:  false, // TODO: Check if file exists
		})
	}

	// Render through UI
	return g.ui.RenderDryRunPreview(config, targetDir, files)
}
```

### Phase 4: Update Command Handlers

Update `cmd/scaffold/scaffold.go`:

```go
func executeScaffoldGenerate(
	cmd *cobra.Command,
	templateName string,
	targetDir string,
	force bool,
	dryRun bool,
	templateVars map[string]interface{},
) error {
	// Create generator context (for UI)
	genCtx, err := setup.NewGeneratorContext()
	if err != nil {
		return errUtils.Build(errUtils.ErrCreateGeneratorContext).
			WithExplanation("Failed to initialize generator context").
			Err()
	}

	// Create dependencies
	ui := NewProductionUI(genCtx.UI)
	templateLoader := NewProductionTemplateLoader()
	executor := NewProductionTemplateExecutor(genCtx)

	// Create generator with dependency injection
	generator := NewScaffoldGenerator(ui, templateLoader, executor)

	// Execute
	return generator.Generate(GenerateOptions{
		TemplateName: templateName,
		TargetDir:    targetDir,
		Force:        force,
		DryRun:       dryRun,
		Values:       templateVars,
	})
}
```

### Phase 5: Generate Mocks and Write Tests

Create `cmd/scaffold/mocks_test.go`:

```go
//go:generate go run go.uber.org/mock/mockgen -source=interfaces.go -destination=mocks_test.go -package=scaffold

package scaffold
```

Generate mocks:

```bash
cd cmd/scaffold
go generate
```

Create `cmd/scaffold/generator_test.go`:

```go
package scaffold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/generator/templates"
)

func TestScaffoldGenerator_Generate_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockUI := NewMockScaffoldUI(ctrl)
	mockLoader := NewMockTemplateLoader(ctrl)
	mockExecutor := NewMockTemplateExecutor(ctrl)

	// Setup expectations
	testConfig := templates.Configuration{
		Name:  "test-template",
		Files: []templates.File{{Path: "test.txt", Content: "content"}},
	}

	configs := map[string]templates.Configuration{
		"test-template": testConfig,
	}

	mockLoader.EXPECT().LoadTemplates().Return(configs, nil)
	mockLoader.EXPECT().MergeConfiguredTemplates(configs).Return(nil)
	mockUI.EXPECT().Info(gomock.Any()).Return(nil)
	mockExecutor.EXPECT().Generate(testConfig, gomock.Any(), false, gomock.Any()).Return(nil)
	mockUI.EXPECT().Success(gomock.Any()).Return(nil)

	// Create generator
	generator := NewScaffoldGenerator(mockUI, mockLoader, mockExecutor)

	// Execute
	err := generator.Generate(GenerateOptions{
		TemplateName: "test-template",
		TargetDir:    "/tmp/test",
		Force:        false,
		DryRun:       false,
		Values:       map[string]interface{}{},
	})

	// Assert
	require.NoError(t, err)
}

func TestScaffoldGenerator_Generate_DryRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUI := NewMockScaffoldUI(ctrl)
	mockLoader := NewMockTemplateLoader(ctrl)
	mockExecutor := NewMockTemplateExecutor(ctrl)

	testConfig := templates.Configuration{
		Name: "test-template",
		Files: []templates.File{
			{Path: "test.txt", Content: "content"},
		},
	}

	configs := map[string]templates.Configuration{
		"test-template": testConfig,
	}

	mockLoader.EXPECT().LoadTemplates().Return(configs, nil)
	mockLoader.EXPECT().MergeConfiguredTemplates(configs).Return(nil)
	mockUI.EXPECT().RenderDryRunPreview(&testConfig, gomock.Any(), gomock.Any()).Return(nil)

	generator := NewScaffoldGenerator(mockUI, mockLoader, mockExecutor)

	err := generator.Generate(GenerateOptions{
		TemplateName: "test-template",
		TargetDir:    "/tmp/test",
		DryRun:       true,
		Values:       map[string]interface{}{},
	})

	require.NoError(t, err)
}

func TestScaffoldGenerator_Generate_TemplateLoadError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUI := NewMockScaffoldUI(ctrl)
	mockLoader := NewMockTemplateLoader(ctrl)
	mockExecutor := NewMockTemplateExecutor(ctrl)

	// Template loading fails
	mockLoader.EXPECT().LoadTemplates().Return(nil, errors.New("load failed"))

	generator := NewScaffoldGenerator(mockUI, mockLoader, mockExecutor)

	err := generator.Generate(GenerateOptions{
		TemplateName: "test-template",
		TargetDir:    "/tmp/test",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load")
}

func TestScaffoldGenerator_ResolveTargetDirectory(t *testing.T) {
	generator := NewScaffoldGenerator(nil, nil, nil)

	tests := []struct {
		name        string
		targetDir   string
		expectError bool
	}{
		{
			name:        "empty dir",
			targetDir:   "",
			expectError: false,
		},
		{
			name:        "relative path",
			targetDir:   "./test",
			expectError: false,
		},
		{
			name:        "absolute path",
			targetDir:   "/tmp/test",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generator.resolveTargetDirectory(tt.targetDir)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.targetDir != "" {
					assert.True(t, filepath.IsAbs(result))
				}
			}
		})
	}
}
```

## Expected Coverage Improvements

### Before Refactoring
- `cmd/scaffold/scaffold.go`: **50.11%** (225 missed lines)

### After Refactoring
- `cmd/scaffold/scaffold.go`: **70%+** (command handlers only)
- `cmd/scaffold/generator.go`: **90%+** (fully testable business logic)
- `cmd/scaffold/ui_impl.go`: **60-70%** (thin wrappers)
- `cmd/scaffold/template_loader_impl.go`: **80%+** (testable with mocks)

### Total Expected Improvement
- **Current**: 50.11% coverage
- **Target**: 75%+ coverage
- **Improvement**: +25 percentage points

## Implementation Checklist

- [ ] Create `cmd/scaffold/interfaces.go` with all interfaces
- [ ] Create `cmd/scaffold/ui_impl.go` with ProductionUI
- [ ] Create `cmd/scaffold/template_loader_impl.go` with ProductionTemplateLoader
- [ ] Create `cmd/scaffold/template_executor_impl.go` with ProductionTemplateExecutor
- [ ] Create `cmd/scaffold/generator.go` with ScaffoldGenerator
- [ ] Update `cmd/scaffold/scaffold.go` command handlers to use generator
- [ ] Add `//go:generate` directive for mock generation
- [ ] Run `go generate ./cmd/scaffold`
- [ ] Create `cmd/scaffold/generator_test.go` with comprehensive tests
- [ ] Run tests and verify coverage: `go test -cover ./cmd/scaffold`
- [ ] Update existing tests in `scaffold_test.go` if needed
- [ ] Add integration tests for command execution
- [ ] Document the new architecture in comments
- [ ] Update CLAUDE.md with scaffold as example of good architecture

## Benefits

1. **Testability**: All business logic can be unit tested with mocks
2. **Maintainability**: Clear separation of concerns
3. **Extensibility**: Easy to add new features without touching UI code
4. **Follows Best Practices**: Aligns with CLAUDE.md architectural patterns
5. **Mock-based Testing**: Fast, deterministic tests without UI dependencies

## Timeline Estimate

- **Phase 1-2** (Interfaces & Implementations): 2-3 hours
- **Phase 3** (Business Logic Refactoring): 3-4 hours
- **Phase 4** (Command Handler Updates): 1-2 hours
- **Phase 5** (Mocks & Tests): 3-4 hours
- **Total**: 9-13 hours of focused development

## Risks & Mitigations

**Risk**: Breaking existing functionality during refactoring
**Mitigation**: Keep old functions alongside new ones, migrate gradually, extensive testing

**Risk**: Mock generation issues
**Mitigation**: Use proven `go.uber.org/mock/mockgen`, follow existing patterns in codebase

**Risk**: Test maintenance overhead
**Mitigation**: Keep tests focused on behavior, use table-driven tests, avoid implementation details

## Success Criteria

1. ✅ All files achieve 70%+ coverage
2. ✅ All existing tests continue to pass
3. ✅ New tests provide comprehensive behavior coverage
4. ✅ No breaking changes to CLI interface
5. ✅ Code follows CLAUDE.md architectural patterns
6. ✅ Documentation is updated
