package backend

import (
	"fmt"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/configuration"
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
	if _, owned := info.ComponentSettingsSection["configuration"]; !owned {
		return u.WriteToFileAsJSON(backendFileName, componentBackendConfig, backendFileMode)
	}
	// The write and its receipt share the workdir lock with RemoveHostBackend, so a handoff never
	// removes a backend written after it verified the receipt.
	err = configuration.WithWorkdirLock(workingDir, func() error {
		if err := u.WriteToFileAsJSON(backendFileName, componentBackendConfig, backendFileMode); err != nil {
			return err
		}
		return recordHostBackend(workingDir)
	})
	if err != nil {
		return fmt.Errorf("%w: write %s and record ownership: %w", errUtils.ErrBackendFileGeneration, backendFileName, err)
	}
	return nil
}

const backendFileMode = 0o600
