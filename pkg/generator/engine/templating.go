package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/hairyhenderson/gomplate/v3"
	"github.com/hairyhenderson/gomplate/v3/data"

	"github.com/cloudposse/atmos/pkg/generator/merge"
	"github.com/cloudposse/atmos/pkg/project/config"
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
	return p.ProcessTemplateWithDelimiters(content, targetPath, scaffoldConfig, userValues, []string{"{{", "}}"})
}

// ProcessTemplateWithDelimiters processes Go templates in file content with custom delimiters
func (p *Processor) ProcessTemplateWithDelimiters(content string, targetPath string, scaffoldConfig interface{}, userValues map[string]interface{}, delimiters []string) (string, error) {
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

	// Parse and execute template with custom delimiters
	tmpl, err := template.New("init").Delims(delimiters[0], delimiters[1]).Funcs(funcs).Parse(content)
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

// ProcessFile handles the creation of a single file with full templating support.
func (p *Processor) ProcessFile(file File, targetPath string, force, update bool, scaffoldConfig interface{}, userValues map[string]interface{}) error {
	// Extract delimiters from config
	delimiters := extractDelimiters(scaffoldConfig)

	// Process and validate the file path
	renderedPath, err := p.processFilePath(file.Path, targetPath, scaffoldConfig, userValues, delimiters)
	if err != nil {
		return err
	}

	// Check if file should be skipped
	if p.ShouldSkipFile(renderedPath) {
		return &FileSkippedError{Path: file.Path, RenderedPath: renderedPath}
	}

	// Prepare target path and directory
	fullPath := filepath.Join(targetPath, renderedPath)
	if err := ensureDirectory(fullPath); err != nil {
		return err
	}

	// Handle existing file
	if fileExists(fullPath) {
		return p.handleExistingFile(file, fullPath, targetPath, force, update, scaffoldConfig, userValues, delimiters)
	}

	// Process and write new file
	return p.writeNewFile(file, fullPath, targetPath, scaffoldConfig, userValues, delimiters)
}

// extractDelimiters extracts template delimiters from scaffold config or returns defaults.
func extractDelimiters(scaffoldConfig interface{}) []string {
	delimiters := []string{"{{", "}}"}

	if scaffoldConfig == nil {
		return delimiters
	}

	// Try *config.ScaffoldConfig first (pointer)
	if cfg, ok := scaffoldConfig.(*config.ScaffoldConfig); ok {
		if len(cfg.Delimiters) == 2 {
			return []string{cfg.Delimiters[0], cfg.Delimiters[1]}
		}
	} else if cfg, ok := scaffoldConfig.(config.ScaffoldConfig); ok {
		// Try config.ScaffoldConfig (value)
		if len(cfg.Delimiters) == 2 {
			return []string{cfg.Delimiters[0], cfg.Delimiters[1]}
		}
	} else if scaffoldConfigMap, ok := scaffoldConfig.(map[string]interface{}); ok {
		// Fallback to map handling for backwards compatibility
		if delims, exists := scaffoldConfigMap["delimiters"]; exists {
			if delimsSlice, ok := delims.([]interface{}); ok && len(delimsSlice) == 2 {
				return []string{delimsSlice[0].(string), delimsSlice[1].(string)}
			}
		}
	}

	return delimiters
}

// processFilePath processes the file path as a template and returns the rendered path.
func (p *Processor) processFilePath(filePath, targetPath string, scaffoldConfig interface{}, userValues map[string]interface{}, delimiters []string) (string, error) {
	if userValues == nil {
		return filePath, nil
	}

	renderedPath, err := p.ProcessTemplateWithDelimiters(filePath, targetPath, scaffoldConfig, userValues, delimiters)
	if err != nil {
		return "", fmt.Errorf("failed to process file path template %s: %w", filePath, err)
	}

	return renderedPath, nil
}

// ensureDirectory creates the directory for the given file path if it doesn't exist.
func ensureDirectory(fullPath string) error {
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	return nil
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// handleExistingFile handles the case where the target file already exists.
func (p *Processor) handleExistingFile(file File, fullPath, targetPath string, force, update bool, scaffoldConfig interface{}, userValues map[string]interface{}, delimiters []string) error {
	// Check flags
	if !force && !update {
		return fmt.Errorf("file already exists: %s (use --force to overwrite or --update to merge)", file.Path)
	}

	// Handle update mode (3-way merge)
	if update {
		processedContent, err := p.ProcessTemplateWithDelimiters(file.Content, targetPath, scaffoldConfig, userValues, delimiters)
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

	// force flag is set, allow overwrite by returning nil (caller will write)
	return nil
}

// writeNewFile processes the file content and writes it to disk.
func (p *Processor) writeNewFile(file File, fullPath, targetPath string, scaffoldConfig interface{}, userValues map[string]interface{}, delimiters []string) error {
	// Process content as template if needed
	content, err := p.processFileContent(file, targetPath, scaffoldConfig, userValues, delimiters)
	if err != nil {
		return err
	}

	// Write file
	if err := os.WriteFile(fullPath, []byte(content), file.Permissions); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// processFileContent processes file content as a template and validates it.
func (p *Processor) processFileContent(file File, targetPath string, scaffoldConfig interface{}, userValues map[string]interface{}, delimiters []string) (string, error) {
	content := file.Content

	// Only process if user values provided or file is marked as template
	if userValues == nil && !file.IsTemplate {
		return content, nil
	}

	processedContent, err := p.ProcessTemplateWithDelimiters(content, targetPath, scaffoldConfig, userValues, delimiters)
	if err != nil {
		return "", fmt.Errorf("failed to process template for file %s: %w\nTemplate content preview: %s\nUser values: %+v",
			file.Path, err,
			truncateString(content, 200),
			userValues)
	}

	// Validate that the processed content contains no unprocessed templates
	if err := p.ValidateNoUnprocessedTemplatesWithDelimiters(processedContent, delimiters); err != nil {
		return "", fmt.Errorf("generated file %s contains unprocessed template syntax: %w", file.Path, err)
	}

	return processedContent, nil
}

// mergeFile attempts a 3-way merge for existing files
func (p *Processor) mergeFile(existingPath string, file File, targetPath string) error {
	// Read existing file content
	existingContent, err := os.ReadFile(existingPath)
	if err != nil {
		return fmt.Errorf("failed to read existing file: %w", err)
	}

	// Process new content with default delimiters
	newContent := file.Content
	if file.IsTemplate {
		processedContent, err := p.ProcessTemplateWithDelimiters(newContent, targetPath, nil, nil, []string{"{{", "}}"})
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

// ContainsUnprocessedTemplatesWithDelimiters checks if the given content contains unprocessed template syntax with custom delimiters
func (p *Processor) ContainsUnprocessedTemplatesWithDelimiters(content string, delimiters []string) bool {
	if len(delimiters) != 2 {
		// Fall back to default delimiters if invalid
		return p.ContainsUnprocessedTemplates(content)
	}
	return strings.Contains(content, delimiters[0]) && strings.Contains(content, delimiters[1])
}

// ValidateNoUnprocessedTemplates validates that the processed content doesn't contain unprocessed template syntax
func (p *Processor) ValidateNoUnprocessedTemplates(content string) error {
	if p.ContainsUnprocessedTemplates(content) {
		return fmt.Errorf("generated content contains unprocessed template syntax: %s", truncateString(content, 200))
	}
	return nil
}

// ValidateNoUnprocessedTemplatesWithDelimiters validates that the processed content doesn't contain unprocessed template syntax with custom delimiters
func (p *Processor) ValidateNoUnprocessedTemplatesWithDelimiters(content string, delimiters []string) error {
	if p.ContainsUnprocessedTemplatesWithDelimiters(content, delimiters) {
		return fmt.Errorf("generated content contains unprocessed template syntax with delimiters %v: %s", delimiters, truncateString(content, 200))
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
