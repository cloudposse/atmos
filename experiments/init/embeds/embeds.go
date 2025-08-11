package embeds

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

//go:embed templates/*
//go:embed templates/editorconfig/.editorconfig
//go:embed templates/gitignore/.gitignore
var templateFS embed.FS

// Configuration represents an initialization configuration
type Configuration struct {
	Name        string
	Description string
	Files       []File
	README      string
	TargetDir   string // Optional default target directory
	TemplateID  string // Template identifier (e.g., "default", "rich-project")
}

// File represents a file to be created during initialization
type File struct {
	Path        string
	Content     string
	IsTemplate  bool
	Permissions fs.FileMode
}

// readTemplateFromDir reads a template file from a specific directory
func readTemplateFromDir(dir, name string) (string, error) {
	content, err := templateFS.ReadFile(filepath.Join("templates", dir, name))
	if err != nil {
		return "", fmt.Errorf("failed to read template %s/%s: %w", dir, name, err)
	}
	return string(content), nil
}

// readProjectConfigMetadata reads name, description, target_dir, and template_id from project-config.yaml
func readProjectConfigMetadata(dir string) (name, description, targetDir, templateID string, err error) {
	// Try to read project-config.yaml
	content, err := templateFS.ReadFile(filepath.Join("templates", dir, "project-config.yaml"))
	if err != nil {
		// If no project-config.yaml exists, return empty strings
		return "", "", "", "", nil
	}

	// Simple struct to extract just name, description, target_dir, and template_id
	var config struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		TargetDir   string `yaml:"target_dir"`
		TemplateID  string `yaml:"template_id"`
	}

	if err := yaml.Unmarshal(content, &config); err != nil {
		return "", "", "", "", fmt.Errorf("failed to unmarshal project config: %w", err)
	}

	// If template_id is not specified, use the directory name as fallback
	if config.TemplateID == "" {
		config.TemplateID = dir
	}

	return config.Name, config.Description, config.TargetDir, config.TemplateID, nil
}

// readAllFilesFromDir reads all files from a directory and returns them as File structs
func readAllFilesFromDir(dir string) ([]File, error) {
	var files []File

	// Walk through the directory
	err := fs.WalkDir(templateFS, filepath.Join("templates", dir), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Read file content
		content, err := templateFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", path, err)
		}

		// Get relative path from the template directory
		relPath, err := filepath.Rel(filepath.Join("templates", dir), path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}

		// Determine if it's a template based on file extension
		isTemplate := strings.HasSuffix(relPath, ".md") || strings.HasSuffix(relPath, ".yaml") || strings.HasSuffix(relPath, ".yml") || strings.HasSuffix(relPath, ".tf")

		// Set permissions based on file type
		permissions := fs.FileMode(0644)
		if strings.HasSuffix(relPath, ".sh") {
			permissions = 0755
		}

		files = append(files, File{
			Path:        relPath,
			Content:     string(content),
			IsTemplate:  isTemplate,
			Permissions: permissions,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory %s: %w", dir, err)
	}

	return files, nil
}

// GetAvailableConfigurations returns all available initialization configurations
func GetAvailableConfigurations() (map[string]Configuration, error) {
	configs := make(map[string]Configuration)

	// Discover all template directories dynamically
	templateDirs, err := discoverTemplateDirectories()
	if err != nil {
		return nil, fmt.Errorf("failed to discover template directories: %w", err)
	}

	// Process each discovered template directory
	for _, dir := range templateDirs {
		config, err := loadConfigurationFromDirectory(dir)
		if err != nil {
			return nil, fmt.Errorf("failed to load configuration from %s: %w", dir, err)
		}
		configs[dir] = config
	}

	// Handle special single-file templates
	singleFileTemplates := []struct {
		name        string
		description string
		filePath    string
		outputPath  string
	}{
		{
			name:        "atmos.yaml",
			description: "Initialize a local atmos CLI configuration file",
			filePath:    "templates/atmos-yaml/atmos.yaml",
			outputPath:  "atmos.yaml",
		},
		{
			name:        ".editorconfig",
			description: "Initialize a local Editor Config file",
			filePath:    "templates/editorconfig/.editorconfig",
			outputPath:  ".editorconfig",
		},
		{
			name:        ".gitignore",
			description: "Initialize a recommend Git ignore file",
			filePath:    "templates/gitignore/.gitignore",
			outputPath:  ".gitignore",
		},
	}

	for _, tmpl := range singleFileTemplates {
		content, err := templateFS.ReadFile(tmpl.filePath)
		if err != nil {
			// Skip if file doesn't exist
			continue
		}

		configs[tmpl.name] = Configuration{
			Name:        tmpl.name,
			Description: tmpl.description,
			Files: []File{
				{
					Path:        tmpl.outputPath,
					Content:     string(content),
					IsTemplate:  true,
					Permissions: 0644,
				},
			},
			TemplateID: tmpl.name,
		}
	}

	return configs, nil
}

// discoverTemplateDirectories finds all template directories in the embedded filesystem
func discoverTemplateDirectories() ([]string, error) {
	var dirs []string

	// Walk through the templates directory
	err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root templates directory
		if path == "templates" {
			return nil
		}

		// Only process directories that contain project-config.yaml or multiple files
		if d.IsDir() {
			// Check if this directory has a project-config.yaml (multi-file template)
			if hasProjectConfig(path) {
				// Get relative path from templates directory
				relPath, err := filepath.Rel("templates", path)
				if err != nil {
					return fmt.Errorf("failed to get relative path for %s: %w", path, err)
				}
				dirs = append(dirs, relPath)
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk templates directory: %w", err)
	}

	return dirs, nil
}

// hasProjectConfig checks if a directory contains a project-config.yaml file
func hasProjectConfig(dirPath string) bool {
	_, err := templateFS.ReadFile(filepath.Join(dirPath, "project-config.yaml"))
	return err == nil
}

// loadConfigurationFromDirectory loads a configuration from a template directory
func loadConfigurationFromDirectory(dir string) (Configuration, error) {
	// Read all files from the directory
	files, err := readAllFilesFromDir(dir)
	if err != nil {
		return Configuration{}, fmt.Errorf("failed to read files from %s: %w", dir, err)
	}

	// Read metadata from project-config.yaml
	name, description, targetDir, templateID, err := readProjectConfigMetadata(dir)
	if err != nil {
		return Configuration{}, fmt.Errorf("failed to read metadata from %s: %w", dir, err)
	}

	// Find README
	var readme string
	for _, file := range files {
		if file.Path == "README.md" {
			readme = file.Content
			break
		}
	}

	// Use metadata if available, otherwise use defaults
	if name == "" {
		name = dir
	}
	if description == "" {
		description = fmt.Sprintf("Template: %s", dir)
	}
	if templateID == "" {
		templateID = dir
	}

	return Configuration{
		Name:        name,
		Description: description,
		Files:       files,
		README:      readme,
		TargetDir:   targetDir,
		TemplateID:  templateID,
	}, nil
}

// GetEmbeddedTemplate retrieves a specific embedded template by name
func GetEmbeddedTemplate(name string) (string, error) {
	content, err := templateFS.ReadFile(filepath.Join("templates", name))
	if err != nil {
		return "", fmt.Errorf("failed to read embedded template %s: %w", name, err)
	}
	return string(content), nil
}
