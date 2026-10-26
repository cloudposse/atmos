package backend

import (
	"path/filepath"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// GenerateFile writes backend.tf.json when enabled, except for Terraform test runs.
// Existing files are preserved when generation is skipped.
func GenerateFile(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	defer perf.Track(atmosConfig, "backend.GenerateFile")()

	// Terraform tests use their own state and do not need a generated backend.
	if info.SubCommand == "test" || !atmosConfig.Components.Terraform.AutoGenerateBackendFile {
		return nil
	}
	// Auto-generate backend file.
	backendFileName := filepath.Join(workingDir, "backend.tf.json")
	log.Debug("Writing the backend config to file.", "file", backendFileName)
	if info.DryRun {
		return nil
	}
	componentBackendConfig, err := Config(info.ComponentBackendType, info.ComponentBackendSection, info.TerraformWorkspace)
	if err != nil {
		return err
	}
	return u.WriteToFileAsJSON(backendFileName, componentBackendConfig, backendFileMode)
}

const backendFileMode = 0o600
