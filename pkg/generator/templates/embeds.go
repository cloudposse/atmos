package templates

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// MagicCommentMaxLines is the number of lines to scan for atmos:template magic comments.
	magicCommentMaxLines = 10
	// DefaultFilePermissions is the default permission for generated files.
	defaultFilePermissions = 0o644
)

// Configuration represents a template configuration loaded from the embedded filesystem.
// It contains metadata about a scaffold template including its name, description, and
// the collection of files to be generated. Key fields:
//   - Name: Human-readable template name
//   - Description: Brief description of the template's purpose
//   - TemplateID: Unique identifier for the template
//   - TargetDir: Default target directory for template output
//   - Files: Collection of files/directories in this template
//   - README: Optional README content for the template
type Configuration struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	TemplateID  string `yaml:"template_id"`
	TargetDir   string `yaml:"target_dir"`
	Files       []File `yaml:"files"`
	README      string `yaml:"readme"`
}

// File represents an embedded template file used by the generator.
// This struct is specifically for files loaded from the embedded filesystem (embed.FS)
// and differs from other File types in the codebase:
//   - engine.File: Used for runtime template processing and file generation
//   - types.File: Used for API/DTO models and general file representations
//
// Use this File type when:
//   - Working with generator templates loaded from embedded assets
//   - Loading template configurations from the embedded filesystem
//   - Representing files before they are processed by the templating engine
//
// Key fields:
//   - Path: Relative path within the template
//   - Content: Raw file content (may contain template syntax if IsTemplate is true)
//   - IsDirectory: Whether this represents a directory structure
//   - IsTemplate: Whether content should be processed as a Go template
//   - Permissions: Unix file permissions to apply when creating files
type File struct {
	Path        string      `yaml:"path"`
	Content     string      `yaml:"content"`
	IsDirectory bool        `yaml:"is_directory"`
	IsTemplate  bool        `yaml:"is_template"`
	Permissions os.FileMode `yaml:"permissions"`
}

// GetAvailableConfigurations loads available template configurations from the embedded templates filesystem.
// It scans the templates directory, loads each template configuration, and returns a map of template name
// to Configuration.
//
// Returns:
//   - map[string]Configuration: Map of template names to their configurations
//   - error: Non-nil if reading the templates directory fails
//
// The function returns an error only when the templates directory itself cannot be read.
// Individual template loading errors are silently skipped to allow partial success.
func GetAvailableConfigurations() (map[string]Configuration, error) {
	defer perf.Track(nil, "templates.GetAvailableConfigurations")()

	configs := make(map[string]Configuration)

	// Read templates directory
	// Note: embed.FS always uses forward slashes, even on Windows
	templatesDir := "templates"
	entries, err := generator.Templates.ReadDir(templatesDir)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrReadTemplatesDirectory).
			WithExplanation("Failed to read embedded templates directory").
			WithHint("Embedded templates may be missing from the binary").
			WithHint("Try rebuilding Atmos: `make build`").
			WithHint("Check that templates are properly embedded with `go:embed`").
			WithContext("templates_dir", templatesDir).
			WithExitCode(1).
			Err()
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Use path.Join (forward slashes) not filepath.Join for embed.FS.
			templatePath := path.Join(templatesDir, entry.Name()) //nolint:forbidigo // embed.FS always uses forward slashes
			config, err := loadConfiguration(templatePath)
			if err != nil {
				// Skip templates that can't be loaded
				continue
			}
			configs[entry.Name()] = *config
		}
	}

	return configs, nil
}

// loadConfiguration loads a template configuration from the embedded filesystem.
func loadConfiguration(templatePath string) (*Configuration, error) {
	// Read all files in the template directory
	files, err := readTemplateFiles(templatePath)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrReadTemplateFiles).
			WithExplanationf("Cannot read template directory: `%s`", templatePath).
			WithHint("Template may be corrupted or missing files").
			WithHint("Try rebuilding Atmos: `make build`").
			WithContext("template_path", templatePath).
			WithExitCode(1).
			Err()
	}

	// Find README if it exists
	var readmeContent string
	// Use path.Join (forward slashes) not filepath.Join for embed.FS.
	readmePath := path.Join(templatePath, "README.md") //nolint:forbidigo // embed.FS always uses forward slashes
	if data, err := generator.Templates.ReadFile(readmePath); err == nil {
		readmeContent = string(data)
	}

	// Default metadata
	templateName := path.Base(templatePath)
	configName := templateName
	configDescription := fmt.Sprintf("%s template", templateName)
	templateID := templateName

	// Try to load scaffold.yaml for metadata
	// Use path.Join (forward slashes) not filepath.Join for embed.FS.
	scaffoldPath := path.Join(templatePath, "scaffold.yaml") //nolint:forbidigo // embed.FS always uses forward slashes
	if data, err := generator.Templates.ReadFile(scaffoldPath); err == nil {
		// Parse scaffold.yaml to extract metadata (basic parsing)
		// For now, just use defaults since we don't have full scaffold parsing
		_ = data // TODO: Parse scaffold config for name/description
	}

	return &Configuration{
		Name:        configName,
		Description: configDescription,
		TemplateID:  templateID,
		Files:       files,
		README:      readmeContent,
	}, nil
}

// templateMagicCommentPattern matches magic comments that indicate a file should be treated as a template.
// Supported formats:
//   - # atmos:template (shell, Python, Ruby, YAML, etc.)
//   - // atmos:template (Go, JavaScript, C++, etc.)
//   - /* atmos:template */ (C-style block comments)
//   - <!-- atmos:template --> (HTML, XML, Markdown)
//
// The magic comment must appear within the first 10 lines of the file and is case-insensitive.
var templateMagicCommentPattern = regexp.MustCompile(`(?i)(?:^|//|#|/\*|<!--)\s*atmos:template\s*(?:\*/|-->)?`)

// hasTemplateMagicComment checks if the content contains an atmos:template magic comment
// in the first magicCommentMaxLines lines of the file.
func hasTemplateMagicComment(content string) bool {
	lines := strings.Split(content, "\n")
	if len(lines) > magicCommentMaxLines {
		lines = lines[:magicCommentMaxLines]
	}

	for _, line := range lines {
		if templateMagicCommentPattern.MatchString(strings.TrimSpace(line)) {
			return true
		}
	}

	return false
}

// stripTemplateMagicComment removes the atmos:template magic comment from the content.
// This ensures the magic comment doesn't appear in the generated output.
func stripTemplateMagicComment(content string) string {
	lines := strings.Split(content, "\n")
	var filtered []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip lines that contain only the magic comment
		if templateMagicCommentPattern.MatchString(trimmed) {
			// Remove the magic comment from the original line (not just the trimmed copy)
			cleaned := templateMagicCommentPattern.ReplaceAllString(line, "")
			// Create a trimmed copy for emptiness checks
			trimmedCleaned := strings.TrimSpace(cleaned)
			// If nothing remains after removing the magic comment, skip this line
			if trimmedCleaned == "" || trimmedCleaned == "//" || trimmedCleaned == "#" || trimmedCleaned == "/*" || trimmedCleaned == "*/" || trimmedCleaned == "<!--" || trimmedCleaned == "-->" {
				continue
			}
			// Preserve leading indentation but remove trailing whitespace after magic comment removal
			cleaned = strings.TrimRight(cleaned, " \t")
			// Append the cleaned line with leading indentation preserved
			filtered = append(filtered, cleaned)
			continue
		}
		filtered = append(filtered, line)
	}

	return strings.Join(filtered, "\n")
}

// readTemplateFiles recursively reads all files from a template directory.
func readTemplateFiles(templatePath string) ([]File, error) {
	var files []File

	entries, err := generator.Templates.ReadDir(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory '%s': %w", templatePath, err)
	}

	for _, entry := range entries {
		// Use path.Join (forward slashes) not filepath.Join for embed.FS.
		filePath := path.Join(templatePath, entry.Name()) //nolint:forbidigo // embed.FS always uses forward slashes

		var entryFiles []File
		var err error

		if entry.IsDir() {
			entryFiles, err = processDirectoryEntry(templatePath, filePath, entry.Name())
		} else {
			entryFiles, err = processFileEntry(templatePath, filePath, entry.Name())
		}

		if err != nil {
			return nil, err
		}

		files = append(files, entryFiles...)
	}

	return files, nil
}

// processDirectoryEntry processes a directory entry and returns all files within it.
func processDirectoryEntry(templatePath, filePath, entryName string) ([]File, error) {
	var files []File

	// Add directory entry
	files = append(files, File{
		Path:        strings.TrimPrefix(filePath, templatePath+"/"),
		Content:     "",
		IsDirectory: true,
		IsTemplate:  false,
		Permissions: 0o755,
	})

	// Recursively read directory contents
	subFiles, err := readTemplateFiles(filePath)
	if err != nil {
		return nil, err
	}

	// Prepend directory path to sub-files
	for _, subFile := range subFiles {
		// Use path.Join (forward slashes) for consistency in File.Path.
		subFile.Path = path.Join(entryName, subFile.Path) //nolint:forbidigo // embed.FS always uses forward slashes
		files = append(files, subFile)
	}

	return files, nil
}

// processFileEntry processes a file entry and returns the file with its content.
func processFileEntry(templatePath, filePath, entryName string) ([]File, error) {
	// Read file content
	content, err := generator.Templates.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s': %w", filePath, err)
	}

	contentStr := string(content)

	// Determine if file is a template using multiple detection methods:
	// 1. Files with .tmpl extension are always treated as templates
	// 2. Files with atmos:template magic comment in first 10 lines
	//
	// This avoids false positives from files that incidentally contain "{{"
	// (e.g., JSON, code examples, documentation) while allowing explicit
	// template marking via magic comments.
	isTemplate := strings.HasSuffix(entryName, ".tmpl") || hasTemplateMagicComment(contentStr)

	// Strip the magic comment from the content if present
	// This ensures it doesn't appear in generated output
	if hasTemplateMagicComment(contentStr) {
		contentStr = stripTemplateMagicComment(contentStr)
	}

	file := File{
		Path:        strings.TrimPrefix(filePath, templatePath+"/"),
		Content:     contentStr,
		IsDirectory: false,
		IsTemplate:  isTemplate,
		Permissions: defaultFilePermissions,
	}

	return []File{file}, nil
}

// HasScaffoldConfig checks if the configuration has a scaffold.yaml file.
func HasScaffoldConfig(files []File) bool {
	defer perf.Track(nil, "templates.HasScaffoldConfig")()

	for _, file := range files {
		if file.Path == "scaffold.yaml" && !file.IsDirectory {
			return true
		}
	}
	return false
}
