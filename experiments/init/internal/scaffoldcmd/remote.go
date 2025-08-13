package scaffoldcmd

import (
	"fmt"
	"io"
	"os"

	"github.com/cloudposse/atmos/experiments/init/internal/ui"
	"github.com/hashicorp/go-getter"
)

// generateFromRemote handles generation from a remote Git repository
func generateFromRemote(templateURL, targetPath string, force, update, useDefaults bool, cmdValues map[string]interface{}, ui *ui.InitUI, delimiters []string) error {
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
	// For remote templates, use the URL as the template key
	templateKey := templateURL
	config, err := loadLocalTemplate(tempDir, templateKey)
	if err != nil {
		return fmt.Errorf("failed to load template from remote repository: %w", err)
	}

	// Execute the template generation using the existing UI logic with delimiters
	return ui.ExecuteWithDelimiters(*config, targetPath, force, update, useDefaults, cmdValues, delimiters)
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
