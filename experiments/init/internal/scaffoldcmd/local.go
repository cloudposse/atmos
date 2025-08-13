package scaffoldcmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/experiments/init/embeds"
	"github.com/cloudposse/atmos/experiments/init/internal/config"
)

// loadLocalTemplate loads a template configuration from a local filesystem path
func loadLocalTemplate(templatePath string) (*embeds.Configuration, error) {
	// Check if template directory exists
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("template directory does not exist: %s", templatePath)
	}

	// Check for scaffold.yaml (optional)
	scaffoldConfigPath := filepath.Join(templatePath, config.ScaffoldConfigFileName)
	var scaffoldConfig *config.ScaffoldConfig
	var templateID string

	if _, err := os.Stat(scaffoldConfigPath); err == nil {
		// scaffold.yaml exists, read and parse it
		scaffoldConfig, err = config.LoadScaffoldConfigFromFile(scaffoldConfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load %s: %w", config.ScaffoldConfigFileName, err)
		}

		templateID = scaffoldConfig.Name
	}

	// Use directory name as template ID if not set
	if templateID == "" {
		templateID = filepath.Base(templatePath)
	}

	// Read all files from template directory
	files, err := readTemplateFiles(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template files: %w", err)
	}

	// Find README if it exists
	var readmeContent string
	readmePath := filepath.Join(templatePath, "README.md")
	if _, err := os.Stat(readmePath); err == nil {
		readmeData, err := os.ReadFile(readmePath)
		if err == nil {
			readmeContent = string(readmeData)
		}
	}

	// Create configuration
	config := &embeds.Configuration{
		Name:        templateID,
		Description: "Template from local directory",
		TemplateID:  templateID,
		Files:       files,
		README:      readmeContent,
	}

	// Update name and description if scaffold config exists
	if scaffoldConfig != nil {
		if scaffoldConfig.Name != "" {
			config.Name = scaffoldConfig.Name
		}
		if scaffoldConfig.Description != "" {
			config.Description = scaffoldConfig.Description
		}
	}

	return config, nil
}

// readTemplateFiles reads all files from a template directory
func readTemplateFiles(templatePath string) ([]embeds.File, error) {
	var files []embeds.File

	err := filepath.WalkDir(templatePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the template directory itself
		if path == templatePath {
			return nil
		}

		// Skip .git directory
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
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
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", path, err)
		}

		// Get file info to read actual permissions
		fileInfo, err := d.Info()
		if err != nil {
			return fmt.Errorf("failed to get file info for %s: %w", path, err)
		}

		// Get the actual file permissions
		permissions := fileInfo.Mode().Perm()

		// Include scaffold.yaml but mark it as schema-only
		if relPath == config.ScaffoldConfigFileName {
			file := embeds.File{
				Path:        relPath,
				Content:     string(content),
				Permissions: permissions,
			}
			files = append(files, file)
			return nil
		}

		// Convert Windows path separators to forward slashes for consistency
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		// Check if the path contains template syntax
		isTemplate := strings.Contains(relPath, "{{") && strings.Contains(relPath, "}}")

		file := embeds.File{
			Path:        relPath,
			Content:     string(content),
			Permissions: permissions,
			IsTemplate:  isTemplate,
		}

		files = append(files, file)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk template directory: %w", err)
	}

	return files, nil
}
