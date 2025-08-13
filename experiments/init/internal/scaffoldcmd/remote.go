package scaffoldcmd

import (
	"fmt"
	"io"
	"os"

	"github.com/cloudposse/atmos/experiments/init/internal/ui"
	"github.com/hashicorp/go-getter"
	"github.com/spf13/viper"
)

// generateFromRemote handles generation from a remote Git repository
func generateFromRemote(templateURL, targetPath string, force, update, useDefaults bool, cmdValues map[string]interface{}, ui *ui.InitUI) error {
	// Create temporary directory for cloning
	tempDir, err := os.MkdirTemp("", "atmos-scaffold-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Clone the repository to temporary directory
	fmt.Printf("Cloning template from %s...\n", templateURL)

	// Configure go-getter client
	client := &getter.Client{
		Src:  templateURL,
		Dst:  tempDir,
		Mode: getter.ClientModeDir,
		Options: []getter.ClientOption{
			getter.WithProgress(defaultProgressBar()),
		},
	}

	// Clone the repository
	if err := client.Get(); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	// Load template from the cloned directory
	config, err := loadLocalTemplate(tempDir)
	if err != nil {
		return fmt.Errorf("failed to load template from remote repository: %w", err)
	}

	// Read scaffold configuration from atmos.yaml
	scaffoldConfig, err := readScaffoldConfig(targetPath)
	if err != nil {
		return fmt.Errorf("failed to read scaffold config: %w", err)
	}

	// Process scaffold templates
	if err := processScaffoldTemplates(scaffoldConfig, targetPath); err != nil {
		return fmt.Errorf("failed to process scaffold templates: %w", err)
	}

	// Execute the template generation using the existing UI logic
	return ui.Execute(*config, targetPath, force, update, useDefaults, cmdValues)
}

// readScaffoldConfig reads the scaffold configuration from atmos.yaml
func readScaffoldConfig(targetPath string) (map[string]interface{}, error) {
	// Create a new Viper instance for reading the atmos.yaml
	v := viper.New()
	v.SetConfigName("atmos")
	v.SetConfigType("yaml")
	v.AddConfigPath(targetPath)

	// Read the configuration
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read atmos.yaml: %w", err)
	}

	// Check if scaffold section exists
	if !v.IsSet("scaffold") {
		return nil, fmt.Errorf("no scaffold section found in atmos.yaml")
	}

	// Get the scaffold configuration
	scaffoldConfig := v.Get("scaffold")
	if scaffoldConfig == nil {
		return nil, fmt.Errorf("scaffold section is empty")
	}

	// Convert to map[string]interface{} for easier handling
	scaffoldMap, ok := scaffoldConfig.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("scaffold section is not a valid configuration")
	}

	return scaffoldMap, nil
}

// processScaffoldTemplates processes the scaffold templates from the configuration
func processScaffoldTemplates(scaffoldConfig map[string]interface{}, targetPath string) error {
	// Get the templates section
	templates, ok := scaffoldConfig["templates"]
	if !ok {
		return fmt.Errorf("no templates section found in scaffold configuration")
	}

	templatesMap, ok := templates.(map[string]interface{})
	if !ok {
		return fmt.Errorf("templates section is not a valid configuration")
	}

	// Process each template
	for templateName, templateConfig := range templatesMap {
		if err := processTemplate(templateName, templateConfig, targetPath); err != nil {
			return fmt.Errorf("failed to process template %s: %w", templateName, err)
		}
	}

	return nil
}



// processTemplate processes a single scaffold template
func processTemplate(templateName string, templateConfig interface{}, targetPath string) error {
	// Convert template config to map
	templateMap, ok := templateConfig.(map[string]interface{})
	if !ok {
		return fmt.Errorf("template configuration is not valid")
	}

	// Extract template properties
	source, ok := templateMap["source"].(string)
	if !ok {
		return fmt.Errorf("template %s missing source", templateName)
	}

	// TODO: Implement template processing logic
	// This would involve:
	// 1. Using go-getter to download the template
	// 2. Processing template variables
	// 3. Rendering the template to the target directory

	fmt.Printf("Processing template %s from %s\n", templateName, source)

	return nil
}

// progressTracker implements getter.ProgressTracker
type progressTracker struct{}

func (p *progressTracker) TrackProgress(src string, currentSize, totalSize int64, stream io.ReadCloser) (body io.ReadCloser) {
	if totalSize > 0 {
		percentage := float64(currentSize) / float64(totalSize) * 100
		fmt.Printf("\rDownloading: %.1f%% (%d/%d bytes)", percentage, currentSize, totalSize)
		if currentSize >= totalSize {
			fmt.Println() // New line when complete
		}
	}
	return stream
}

// defaultProgressBar provides a simple progress indicator for go-getter
func defaultProgressBar() getter.ProgressTracker {
	return &progressTracker{}
}
