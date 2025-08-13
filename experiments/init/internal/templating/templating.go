package templating

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/cloudposse/atmos/experiments/init/internal/merge"
	"github.com/hairyhenderson/gomplate/v3"
	"github.com/hairyhenderson/gomplate/v3/data"
)

// File represents a file to be processed
type File struct {
	Path        string
	Content     string
	IsTemplate  bool
	Permissions os.FileMode
}

// FileSkippedError represents when a file is intentionally skipped
type FileSkippedError struct {
	Path         string
	RenderedPath string
}

func (e *FileSkippedError) Error() string {
	return fmt.Sprintf("file skipped: %s (rendered as: %s)", e.Path, e.RenderedPath)
}

// Processor handles template processing for the init command
type Processor struct {
	merger *merge.ThreeWayMerger
}

// NewProcessor creates a new template processor
func NewProcessor() *Processor {
	return &Processor{
		merger: merge.NewThreeWayMerger(50), // Default 50% threshold
	}
}

// SetMaxChanges sets the maximum number of changes allowed for 3-way merge
func (p *Processor) SetMaxChanges(thresholdPercent int) {
	p.merger = merge.NewThreeWayMerger(thresholdPercent)
}

// ProcessTemplate processes Go templates in file content
func (p *Processor) ProcessTemplate(content string, targetPath string, scaffoldConfig interface{}, userValues map[string]interface{}) (string, error) {
	// Create template data with rich configuration
	templateData := map[string]interface{}{
		"TemplateName":        filepath.Base(targetPath),
		"TemplateDescription": "An Atmos scaffold template for managing infrastructure as code",
		"ScaffoldPath":        targetPath,
		"Config":              userValues, // Access config values via .Config.Foobar
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

// Merge performs a 3-way merge using the internal merger
func (p *Processor) Merge(existingContent, newContent, fileName string) (string, error) {
	return p.merger.Merge(existingContent, newContent, fileName)
}

// ProcessFile handles the creation of a single file with full templating support
func (p *Processor) ProcessFile(file File, targetPath string, force, update bool, scaffoldConfig interface{}, userValues map[string]interface{}) error {
	// Process the file path as a template if user values are provided
	renderedPath := file.Path
	if userValues != nil {
		var err error
		renderedPath, err = p.ProcessTemplate(file.Path, targetPath, scaffoldConfig, userValues)
		if err != nil {
			return fmt.Errorf("failed to process file path template %s: %w", file.Path, err)
		}
	}

	// Check if the rendered path should be skipped
	if p.ShouldSkipFile(renderedPath) {
		return &FileSkippedError{Path: file.Path, RenderedPath: renderedPath}
	}

	// Create full file path with rendered path
	fullPath := filepath.Join(targetPath, renderedPath)

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
			processedContent, err := p.ProcessTemplate(file.Content, targetPath, scaffoldConfig, userValues)
			if err != nil {
				return fmt.Errorf("failed to process template for file %s: %w", file.Path, err)
			}

			// Create a temporary file with processed content for merging
			tempFile := file
			tempFile.Content = processedContent

			if err := p.mergeFile(fullPath, tempFile, targetPath); err != nil {
				return fmt.Errorf("failed to merge file %s: %w", file.Path, err)
			}
			return nil
		}
		// force flag is set, continue to overwrite
	}

	// Process content as template if user values are provided or if file is marked as template
	content := file.Content
	if userValues != nil || file.IsTemplate {
		processedContent, err := p.ProcessTemplate(content, targetPath, scaffoldConfig, userValues)
		if err != nil {
			// Add detailed debugging information
			return fmt.Errorf("failed to process template for file %s: %w\nTemplate content preview: %s\nUser values: %+v",
				file.Path, err,
				truncateString(content, 200),
				userValues)
		}
		content = processedContent

		// Validate that the processed content contains no unprocessed templates
		if err := p.ValidateNoUnprocessedTemplates(content); err != nil {
			return fmt.Errorf("generated file %s contains unprocessed template syntax: %w", file.Path, err)
		}
	}

	// Write file
	if err := os.WriteFile(fullPath, []byte(content), file.Permissions); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// mergeFile attempts a 3-way merge for existing files
func (p *Processor) mergeFile(existingPath string, file File, targetPath string) error {
	// Read existing file content
	existingContent, err := os.ReadFile(existingPath)
	if err != nil {
		return fmt.Errorf("failed to read existing file: %w", err)
	}

	// Process new content
	newContent := file.Content
	if file.IsTemplate {
		processedContent, err := p.ProcessTemplate(newContent, targetPath, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to process template: %w", err)
		}
		newContent = processedContent
	}

	// Perform 3-way merge using the merge package
	mergedContent, err := p.merger.Merge(string(existingContent), newContent, file.Path)
	if err != nil {
		return fmt.Errorf("failed to perform 3-way merge: %w", err)
	}

	// Write merged content
	if err := os.WriteFile(existingPath, []byte(mergedContent), file.Permissions); err != nil {
		return fmt.Errorf("failed to write merged file: %w", err)
	}

	return nil
}

// ShouldSkipFile determines if a file should be skipped based on its rendered path
func (p *Processor) ShouldSkipFile(renderedPath string) bool {
	// Skip if the path is empty, "false", "null", or "<no value>"
	if renderedPath == "" || renderedPath == "false" || renderedPath == "null" || renderedPath == "<no value>" {
		return true
	}

	// Skip if the path contains empty segments (e.g., "foo//bar" or "/foo" or "foo/")
	// Check for double slashes which indicate empty segments
	if strings.Contains(renderedPath, "//") {
		return true
	}

	// Skip if the path starts or ends with a slash (empty segment)
	if strings.HasPrefix(renderedPath, "/") || strings.HasSuffix(renderedPath, "/") {
		return true
	}

	// Split by path separator and check for empty segments
	segments := strings.Split(renderedPath, string(os.PathSeparator))
	for _, segment := range segments {
		if segment == "" {
			return true
		}
	}

	return false
}

// ContainsUnprocessedTemplates checks if the given content contains unprocessed template syntax
func (p *Processor) ContainsUnprocessedTemplates(content string) bool {
	return strings.Contains(content, "{{") && strings.Contains(content, "}}")
}

// ValidateNoUnprocessedTemplates validates that the processed content doesn't contain unprocessed template syntax
func (p *Processor) ValidateNoUnprocessedTemplates(content string) error {
	if p.ContainsUnprocessedTemplates(content) {
		return fmt.Errorf("generated content contains unprocessed template syntax: %s", truncateString(content, 200))
	}
	return nil
}

// truncateString truncates a string to the specified length and adds "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
