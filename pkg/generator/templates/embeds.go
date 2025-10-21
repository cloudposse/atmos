package templates

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/generator"
)

// Configuration represents a template configuration from embedded FS.
type Configuration struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	TemplateID  string `yaml:"template_id"`
	TargetDir   string `yaml:"target_dir"`
	Files       []File `yaml:"files"`
	README      string `yaml:"readme"`
}

// File represents a file in the embedded template.
type File struct {
	Path        string      `yaml:"path"`
	Content     string      `yaml:"content"`
	IsDirectory bool        `yaml:"is_directory"`
	IsTemplate  bool        `yaml:"is_template"`
	Permissions os.FileMode `yaml:"permissions"`
}

// GetAvailableConfigurations loads available template configurations from embedded FS.
func GetAvailableConfigurations() (map[string]Configuration, error) {
	configs := make(map[string]Configuration)

	// Read templates directory
	templatesDir := "templates"
	entries, err := generator.Templates.ReadDir(templatesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read templates directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			templatePath := filepath.Join(templatesDir, entry.Name())
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
		return nil, fmt.Errorf("failed to read template files: %w", err)
	}

	// Find README if it exists
	var readmeContent string
	readmePath := filepath.Join(templatePath, "README.md")
	if data, err := generator.Templates.ReadFile(readmePath); err == nil {
		readmeContent = string(data)
	}

	// Default metadata
	templateName := filepath.Base(templatePath)
	configName := templateName
	configDescription := fmt.Sprintf("%s template", templateName)
	templateID := templateName

	// Try to load scaffold.yaml for metadata
	scaffoldPath := filepath.Join(templatePath, "scaffold.yaml")
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

// readTemplateFiles recursively reads all files from a template directory.
func readTemplateFiles(templatePath string) ([]File, error) {
	var files []File

	entries, err := generator.Templates.ReadDir(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory '%s': %w", templatePath, err)
	}

	for _, entry := range entries {
		filePath := filepath.Join(templatePath, entry.Name())

		if entry.IsDir() {
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
				subFile.Path = filepath.Join(entry.Name(), subFile.Path)
				files = append(files, subFile)
			}
		} else {
			// Read file content
			content, err := generator.Templates.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to read file '%s': %w", filePath, err)
			}

			// Determine if file is a template (has .tmpl extension or contains template syntax)
			isTemplate := strings.HasSuffix(entry.Name(), ".tmpl") || strings.Contains(string(content), "{{")

			files = append(files, File{
				Path:        strings.TrimPrefix(filePath, templatePath+"/"),
				Content:     string(content),
				IsDirectory: false,
				IsTemplate:  isTemplate,
				Permissions: 0o644,
			})
		}
	}

	return files, nil
}

// HasScaffoldConfig checks if the configuration has a scaffold.yaml file.
func HasScaffoldConfig(files []File) bool {
	for _, file := range files {
		if file.Path == "scaffold.yaml" && !file.IsDirectory {
			return true
		}
	}
	return false
}
