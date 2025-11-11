package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/go-git/go-git/v5"
	"github.com/hairyhenderson/gomplate/v3"
	"github.com/hairyhenderson/gomplate/v3/data"

	"github.com/cloudposse/atmos/pkg/generator/merge"
	"github.com/cloudposse/atmos/pkg/generator/storage"
	"github.com/cloudposse/atmos/pkg/project/config"
)

// File represents a file to be processed by the templating engine.
// It contains the file path (which can itself be a template), the content,
// whether the content should be processed as a template, and the file permissions.
type File struct {
	Path        string      // Path to the file, may contain template syntax for dynamic naming
	Content     string      // File content, processed as template if IsTemplate is true
	IsTemplate  bool        // Whether to process Content as a Go template
	Permissions os.FileMode // Unix file permissions to apply when creating the file
}

// FileSkippedError represents when a file is intentionally skipped during processing.
// Files may be skipped when their rendered path contains empty segments, special values
// like "false" or "<no value>", or other indicators that the file should not be created.
type FileSkippedError struct {
	Path         string // Original file path from the template
	RenderedPath string // Rendered path after template processing
}

// Error returns a formatted error message indicating the file was skipped.
func (e *FileSkippedError) Error() string {
	return fmt.Sprintf("file skipped: %s (rendered as: %s)", e.Path, e.RenderedPath)
}

// Processor handles template processing for scaffold and init commands.
// It provides template rendering with Gomplate and Sprig functions,
// file path templating, and intelligent file merging capabilities.
type Processor struct {
	merger     *merge.ThreeWayMerger
	gitStorage *storage.GitBaseStorage
	targetPath string // Target directory for file generation
}

// NewProcessor creates a new template processor with default settings.
// The processor is initialized with a 50% threshold for 3-way merges,
// meaning merges will be rejected if more than 50% of lines would change.
func NewProcessor() *Processor {
	return &Processor{
		merger: merge.NewThreeWayMerger(50), // Default 50% threshold
	}
}

// SetMaxChanges sets the maximum percentage of changes allowed for 3-way merge operations.
// The thresholdPercent parameter controls how aggressive the merge behavior is:
// a lower value (e.g., 30) is more conservative, while a higher value (e.g., 80)
// allows more extensive changes during merges.
func (p *Processor) SetMaxChanges(thresholdPercent int) {
	p.merger = merge.NewThreeWayMerger(thresholdPercent)
}

// SetupGitStorage initializes git-based storage for 3-way merges.
// The targetPath is used to find the git repository and resolve relative file paths.
// The baseRef specifies which git reference to use as the base for merges (e.g., "main", "v1.0.0").
//
// Returns an error if:
//   - targetPath is not in a git repository
//   - baseRef cannot be resolved
func (p *Processor) SetupGitStorage(targetPath string, baseRef string) error {
	p.targetPath = targetPath

	// Open git repository at target path
	repo, err := git.PlainOpenWithOptions(targetPath, &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	})
	if err != nil {
		// Not in a git repo - this is OK, just means we can't use git-based merging
		return nil
	}

	// Create git storage with base ref
	p.gitStorage = storage.NewGitBaseStorage(repo, baseRef)

	// Validate that base ref exists
	if err := p.gitStorage.ValidateBaseRef(); err != nil {
		return fmt.Errorf("invalid base ref: %w", err)
	}

	return nil
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

// Merge performs a 3-way merge using the internal merger.
// Parameters:
//   - base: The original template content (before any processing)
//   - ours: The user's current version (what exists on disk)
//   - theirs: The new template content (after processing)
//   - fileName: The file name for merge strategy detection
func (p *Processor) Merge(base, ours, theirs, fileName string) (*merge.MergeResult, error) {
	return p.merger.Merge(base, ours, theirs, fileName)
}

// ProcessFile processes a file with templating support, handling path rendering,
// skip logic, and merge/overwrite behavior based on flags.
//
// The method performs the following steps:
//  1. Renders the file path as a template (supports dynamic file naming)
//  2. Checks if the file should be skipped based on the rendered path
//  3. Creates necessary directories
//  4. Handles existing files based on force/update flags:
//     - force: overwrites existing files
//     - update: performs a 3-way merge with existing content
//     - neither: returns an error if file exists
//  5. Processes file content as a template if IsTemplate is true
//  6. Writes the final content to disk with specified permissions
//
// Returns FileSkippedError if the file is intentionally skipped (not considered an error).
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

// mergeFile attempts a 3-way merge for existing files.
func (p *Processor) mergeFile(existingPath string, file File, targetPath string) error {
	// Read existing file content (user's version - "ours")
	existingContent, err := os.ReadFile(existingPath)
	if err != nil {
		return fmt.Errorf("failed to read existing file: %w", err)
	}

	// Determine base content for 3-way merge
	var baseContent string
	if p.gitStorage != nil {
		// Try to load base content from git
		relativePath, err := filepath.Rel(p.targetPath, existingPath)
		if err != nil {
			relativePath = file.Path // Fallback to template path
		}

		gitBase, found, err := p.gitStorage.LoadBase(relativePath)
		if err != nil {
			// Git error - fall back to template content as base
			baseContent = file.Content
		} else if found {
			// Use git version as base
			baseContent = gitBase
		} else {
			// File doesn't exist in base ref
			// This is a user-added file - skip merge, don't touch it
			return nil
		}
	} else {
		// No git storage - use template content as base (legacy behavior)
		baseContent = file.Content
	}

	// Process new template content to get "theirs" version
	newContent := file.Content
	if file.IsTemplate {
		processedContent, err := p.ProcessTemplateWithDelimiters(newContent, targetPath, nil, nil, []string{"{{", "}}"})
		if err != nil {
			return fmt.Errorf("failed to process template: %w", err)
		}
		newContent = processedContent
	}

	// Perform 3-way merge
	// - base: original version from git (or template if no git)
	// - ours: user's current version (existingContent)
	// - theirs: new template version (newContent after processing)
	result, err := p.merger.Merge(baseContent, string(existingContent), newContent, file.Path)
	if err != nil {
		return fmt.Errorf("failed to perform 3-way merge: %w", err)
	}

	// Check for conflicts
	if result.HasConflicts {
		return fmt.Errorf("merge has %d conflicts in file %s - manual resolution required", result.ConflictCount, file.Path)
	}

	// Write merged content
	if err := os.WriteFile(existingPath, []byte(result.Content), file.Permissions); err != nil {
		return fmt.Errorf("failed to write merged file: %w", err)
	}

	return nil
}

// ShouldSkipFile determines if a file should be skipped based on its rendered path.
//
// Files are skipped in the following cases:
//   - Empty path or special values: "", "false", "null", "<no value>"
//   - Paths with empty segments: "foo//bar", "/foo", "foo/"
//   - Paths that would create invalid filesystem entries
//
// This is useful for conditional file generation where template variables
// may evaluate to empty or false values, indicating the file should not be created.
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
