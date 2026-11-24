package backend

import (
	"fmt"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// FileOption configures GenerateFile.
type FileOption func(*fileOptions)

type fileOptions struct {
	// The openTofu function reports whether the configured engine is OpenTofu, which reads `.tofu` files
	// that Terraform ignores. It runs only when a provider-owned module is inspected for its own backend.
	openTofu func() bool
}

// WithOpenTofuDetector sets how GenerateFile tells OpenTofu from Terraform. Without it, the configured
// command's name decides (it contains "tofu").
func WithOpenTofuDetector(detect func() bool) FileOption {
	defer perf.Track(nil, "backend.WithOpenTofuDetector")()

	return func(o *fileOptions) {
		o.openTofu = detect
	}
}

// GenerateFile writes backend.tf.json when enabled, except for Terraform test runs.
// Existing files are preserved when generation is skipped.
func GenerateFile(
	atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string, opts ...FileOption,
) error {
	defer perf.Track(atmosConfig, "backend.GenerateFile")()

	options := fileOptions{openTofu: func() bool { return commandNamesOpenTofu(atmosConfig) }}
	for _, opt := range opts {
		opt(&options)
	}

	// Terraform tests use their own state and do not need a generated backend.
	if info.SubCommand == "test" || !schema.GenerateBackendFile(atmosConfig, info) {
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
	if !schema.ConfigurationOwned(info.ComponentSettingsSection) {
		return u.WriteToFileAsJSON(backendFileName, componentBackendConfig, backendFileMode)
	}
	// A provider-owned unit whose module declares its own backend block gets the settings at init instead
	// of a second backend file.
	settings, isBackend := backendSettings(componentBackendConfig, info.ComponentBackendType)
	if isBackend && moduleDeclaresBackend(workingDir, options.openTofu()) {
		return passBackendToInit(info, workingDir, settings)
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

// commandNamesOpenTofu reports whether the configured Terraform command's executable name contains "tofu".
func commandNamesOpenTofu(atmosConfig *schema.AtmosConfiguration) bool {
	if atmosConfig == nil {
		return false
	}
	return strings.Contains(strings.ToLower(filepath.Base(atmosConfig.Components.Terraform.Command)), "tofu")
}

// backendSettings returns the settings of the `backend` block in a generated backend configuration. A
// `cloud` block has none, since it is not a backend.
func backendSettings(config map[string]any, backendType string) (map[string]any, bool) {
	terraform, _ := config["terraform"].(map[string]any)
	backends, _ := terraform["backend"].(map[string]any)
	settings, ok := backends[backendType].(map[string]any)
	return settings, ok
}
