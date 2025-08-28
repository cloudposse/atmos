package embeds

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/experiments/init/internal/config"
	"github.com/cloudposse/atmos/experiments/init/internal/types"
)

//go:embed templates/*
//go:embed templates/editorconfig/.editorconfig
//go:embed templates/gitignore/.gitignore
var templatesFS embed.FS

// Configuration represents a template configuration
type Configuration = types.Configuration

// File represents a file in a configuration
type File = types.File

// GetAvailableConfigurations returns all available configurations
func GetAvailableConfigurations() (map[string]Configuration, error) {
	configs := make(map[string]Configuration)

	// Read the templates directory
	entries, err := templatesFS.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("failed to read templates directory: %w", err)
	}

	// Process only immediate subdirectories
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Get the configuration name (directory name)
		configName := entry.Name()
		templatePath := filepath.Join("templates", configName)

		// Load the configuration
		config, err := loadConfiguration(templatePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load configuration %s: %w", configName, err)
		}

		// Use template_id as the key if available, otherwise fall back to directory name
		key := config.TemplateID
		if key == "" {
			key = configName
		}

		configs[key] = *config
	}

	return configs, nil
}

// loadConfiguration loads a configuration from a template directory
func loadConfiguration(templatePath string) (*Configuration, error) {
	// Read all files in the template directory
	files, err := readTemplateFiles(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template files: %w", err)
	}

	// Find README if it exists
	var readmeContent string
	readmePath := filepath.Join(templatePath, "README.md")
	if data, err := templatesFS.ReadFile(readmePath); err == nil {
		readmeContent = string(data)
	}

	// Find scaffold.yaml if it exists
	var configName, configDescription, templateID string
	scaffoldPath := filepath.Join(templatePath, config.ScaffoldConfigFileName)
	if data, err := templatesFS.ReadFile(scaffoldPath); err == nil {
		// Parse the scaffold.yaml to get name and description
		scaffoldConfig, err := config.LoadScaffoldConfigFromContent(string(data))
		if err == nil {
			configName = scaffoldConfig.Name
			configDescription = scaffoldConfig.Description
			templateID = scaffoldConfig.TemplateID
		}
	}

	// Use directory name as fallback
	if configName == "" {
		configName = filepath.Base(templatePath)
	}
	if configDescription == "" {
		configDescription = fmt.Sprintf("Template from %s", templatePath)
	}
	if templateID == "" {
		templateID = filepath.Base(templatePath)
	}

	return &Configuration{
		Name:        configName,
		Description: configDescription,
		TemplateID:  templateID,
		Files:       files,
		README:      readmeContent,
	}, nil
}

// readTemplateFiles reads all files from a template directory
func readTemplateFiles(templatePath string) ([]File, error) {
	var files []File

	err := fs.WalkDir(templatesFS, templatePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the template directory itself
		if path == templatePath {
			return nil
		}

		// Skip .git directory
		if d.IsDir() && d.Name() == ".git" {
			return fs.SkipDir
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Calculate relative path from template directory
		relPath, err := filepath.Rel(templatePath, path)
		if err != nil {
			return fmt.Errorf("failed to calculate relative path: %w", err)
		}

		// Read file content
		data, err := templatesFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", path, err)
		}

		// Determine if this is a template file
		isTemplate := strings.Contains(string(data), "{{") && strings.Contains(string(data), "}}")

		// Include scaffold.yaml but mark it as schema-only
		if relPath == config.ScaffoldConfigFileName {
			file := File{
				Path:        relPath,
				Content:     string(data),
				IsTemplate:  false, // Schema file, not a template
				Permissions: 0644,
			}
			files = append(files, file)
			return nil
		}

		// Add the file
		file := File{
			Path:        relPath,
			Content:     string(data),
			IsTemplate:  isTemplate,
			Permissions: 0644,
		}
		files = append(files, file)

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk template files: %w", err)
	}

	return files, nil
}

// HasScaffoldConfig checks if a configuration contains a scaffold.yaml file
func HasScaffoldConfig(files []File) bool {
	for _, file := range files {
		if file.Path == "scaffold.yaml" {
			return true
		}
	}
	return false
}
