package embeds

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

//go:embed templates/atmos-yaml/* templates/default/* templates/demo-helmfile/* templates/demo-localstack/* templates/demo-stacks/* templates/rich-project/*
var templateFS embed.FS

//go:embed templates/editorconfig/.editorconfig
var editorConfigFS embed.FS

//go:embed templates/gitignore/.gitignore
var gitignoreFS embed.FS

// Configuration represents an initialization configuration
type Configuration struct {
	Name        string
	Description string
	Files       []File
	README      string
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

// readProjectConfigMetadata reads name and description from project-config.yaml
func readProjectConfigMetadata(dir string) (name, description string, err error) {
	// Try to read project-config.yaml
	content, err := templateFS.ReadFile(filepath.Join("templates", dir, "project-config.yaml"))
	if err != nil {
		// If no project-config.yaml exists, return empty strings
		return "", "", nil
	}

	// Simple struct to extract just name and description
	var config struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}

	if err := yaml.Unmarshal(content, &config); err != nil {
		return "", "", fmt.Errorf("failed to unmarshal project config: %w", err)
	}

	return config.Name, config.Description, nil
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

	// Default project configuration
	defaultFiles, err := readAllFilesFromDir("default")
	if err != nil {
		return nil, fmt.Errorf("failed to read default configuration: %w", err)
	}

	// Read metadata from project-config.yaml if it exists
	name, description, err := readProjectConfigMetadata("default")
	if err != nil {
		return nil, fmt.Errorf("failed to read default metadata: %w", err)
	}

	// Find README for default config
	var defaultReadme string
	for _, file := range defaultFiles {
		if file.Path == "README.md" {
			defaultReadme = file.Content
			break
		}
	}

	// Use metadata if available, otherwise use defaults
	if name == "" {
		name = "default"
	}
	if description == "" {
		description = "Initialize a typical project for atmos"
	}

	configs["default"] = Configuration{
		Name:        name,
		Description: description,
		Files:       defaultFiles,
		README:      defaultReadme,
	}

	// Rich project configuration
	richProjectFiles, err := readAllFilesFromDir("rich-project")
	if err != nil {
		return nil, fmt.Errorf("failed to read rich-project configuration: %w", err)
	}

	// Read metadata from project-config.yaml
	richName, richDescription, err := readProjectConfigMetadata("rich-project")
	if err != nil {
		return nil, fmt.Errorf("failed to read rich-project metadata: %w", err)
	}

	// Find README for rich project
	var richProjectReadme string
	for _, file := range richProjectFiles {
		if file.Path == "README.md" {
			richProjectReadme = file.Content
			break
		}
	}

	// Use metadata if available, otherwise use defaults
	if richName == "" {
		richName = "rich-project"
	}
	if richDescription == "" {
		richDescription = "Initialize a project with rich configuration and interactive prompts"
	}

	configs["rich-project"] = Configuration{
		Name:        richName,
		Description: richDescription,
		Files:       richProjectFiles,
		README:      richProjectReadme,
	}

	// Individual file configurations
	atmosYAML, err := readTemplateFromDir("atmos-yaml", "atmos.yaml")
	if err != nil {
		return nil, err
	}

	configs["atmos.yaml"] = Configuration{
		Name:        "atmos.yaml",
		Description: "Initialize a local atmos CLI configuration file",
		Files: []File{
			{
				Path:        "atmos.yaml",
				Content:     atmosYAML,
				IsTemplate:  true,
				Permissions: 0644,
			},
		},
	}

	editorConfig, err := editorConfigFS.ReadFile("templates/editorconfig/.editorconfig")
	if err != nil {
		return nil, fmt.Errorf("failed to read editorconfig: %w", err)
	}

	configs[".editorconfig"] = Configuration{
		Name:        ".editorconfig",
		Description: "Initialize a local Editor Config file",
		Files: []File{
			{
				Path:        ".editorconfig",
				Content:     string(editorConfig),
				IsTemplate:  false,
				Permissions: 0644,
			},
		},
	}

	gitignore, err := gitignoreFS.ReadFile("templates/gitignore/.gitignore")
	if err != nil {
		return nil, fmt.Errorf("failed to read gitignore: %w", err)
	}

	configs[".gitignore"] = Configuration{
		Name:        ".gitignore",
		Description: "Initialize a recommend Git ignore file",
		Files: []File{
			{
				Path:        ".gitignore",
				Content:     string(gitignore),
				IsTemplate:  false,
				Permissions: 0644,
			},
		},
	}

	// Demo configurations
	demoStacksFiles, err := readAllFilesFromDir("demo-stacks")
	if err != nil {
		return nil, fmt.Errorf("failed to read demo-stacks configuration: %w", err)
	}

	// Find README for demo-stacks
	var demoStacksReadme string
	for _, file := range demoStacksFiles {
		if file.Path == "README.md" {
			demoStacksReadme = file.Content
			break
		}
	}

	configs["examples/demo-stacks"] = Configuration{
		Name:        "examples/demo-stacks",
		Description: "Demonstration of using Atmos stacks",
		Files:       demoStacksFiles,
		README:      demoStacksReadme,
	}

	demoLocalstackFiles, err := readAllFilesFromDir("demo-localstack")
	if err != nil {
		return nil, fmt.Errorf("failed to read demo-localstack configuration: %w", err)
	}

	// Find README for demo-localstack
	var demoLocalstackReadme string
	for _, file := range demoLocalstackFiles {
		if file.Path == "README.md" {
			demoLocalstackReadme = file.Content
			break
		}
	}

	configs["examples/demo-localstack"] = Configuration{
		Name:        "examples/demo-localstack",
		Description: "Demonstration of using Atmos with localstack",
		Files:       demoLocalstackFiles,
		README:      demoLocalstackReadme,
	}

	demoHelmfileFiles, err := readAllFilesFromDir("demo-helmfile")
	if err != nil {
		return nil, fmt.Errorf("failed to read demo-helmfile configuration: %w", err)
	}

	// Find README for demo-helmfile
	var demoHelmfileReadme string
	for _, file := range demoHelmfileFiles {
		if file.Path == "README.md" {
			demoHelmfileReadme = file.Content
			break
		}
	}

	configs["examples/demo-helmfile"] = Configuration{
		Name:        "examples/demo-helmfile",
		Description: "Demonstration of using Atmos with Helmfile",
		Files:       demoHelmfileFiles,
		README:      demoHelmfileReadme,
	}

	return configs, nil
}

// GetEmbeddedTemplate retrieves an embedded template file by name
func GetEmbeddedTemplate(name string) (string, error) {
	content, err := templateFS.ReadFile(filepath.Join("templates", name))
	if err != nil {
		return "", fmt.Errorf("failed to read embedded template %s: %w", name, err)
	}
	return string(content), nil
}
