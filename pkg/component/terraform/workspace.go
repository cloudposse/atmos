package terraform

import (
	"os"
	"path/filepath"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

// CleanTerraformWorkspace deletes the `.terraform/environment` file from the component directory.
// The `.terraform/environment` file contains the name of the currently selected workspace,
// helping Terraform identify the active workspace context for managing your infrastructure.
// We delete the file to prevent the Terraform prompt asking to select the default or the
// previously used workspace. This happens when different backends are used for the same component.
//
//nolint:nestif // Preserve workspace cleanup and Windows retry behavior during extraction.
func CleanTerraformWorkspace(componentPath string) {
	defer perf.Track(nil, "terraform.CleanTerraformWorkspace")()

	// Get `TF_DATA_DIR` ENV variable, default to `.terraform` if not set
	//nolint:forbidigo // TF_DATA_DIR is a Terraform convention, not an Atmos configuration variable.
	tfDataDir := os.Getenv("TF_DATA_DIR")
	if tfDataDir == "" {
		tfDataDir = ".terraform"
	}

	// Convert relative path to absolute
	if !filepath.IsAbs(tfDataDir) {
		tfDataDir = filepath.Join(componentPath, tfDataDir)
	}

	// Ensure the path is cleaned properly
	tfDataDir = filepath.Clean(tfDataDir)

	// Construct the full file path
	filePath := filepath.Join(tfDataDir, "environment")

	// Check if the file exists before attempting deletion
	if _, err := os.Stat(filePath); err == nil {
		log.Debug("Terraform environment file found. Proceeding with deletion.", "file", filePath)

		// Use retry logic on Windows to handle file locking
		deleteErr := tfoutput.RetryOnWindows(func() error {
			return os.Remove(filePath)
		})

		if deleteErr != nil {
			log.Debug("Failed to delete Terraform environment file.", "file", filePath, "error", deleteErr)
		} else {
			log.Debug("Successfully deleted Terraform environment file.", "file", filePath)
		}
	} else if os.IsNotExist(err) {
		log.Debug("Terraform environment file not found. No action needed.", "file", filePath)
	} else {
		log.Debug("Error checking Terraform environment file.", "file", filePath, "error", err)
	}
}
