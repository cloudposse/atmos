package internal

import (
	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ValidateConfig holds configuration options for Atmos config validation.
type ValidateConfig struct {
	CheckStack bool
	ConfigInfo schema.ConfigAndStacksInfo
}

// ValidateOption is a functional option for configuring validation.
type ValidateOption func(*ValidateConfig)

// WithStackValidation sets whether to check the stacks directory exists.
func WithStackValidation(check bool) ValidateOption {
	return func(cfg *ValidateConfig) {
		cfg.CheckStack = check
	}
}

// WithConfigInfo sets the config info for validation, respecting config selection flags.
func WithConfigInfo(info *schema.ConfigAndStacksInfo) ValidateOption {
	return func(cfg *ValidateConfig) {
		if info != nil {
			cfg.ConfigInfo = *info
		}
	}
}

// ValidateAtmosConfig checks the Atmos configuration and returns an error instead of exiting.
// This makes the function testable by allowing errors to be handled by the caller.
//
// This function validates that the required Atmos configuration exists:
// - By default, checks that the stacks directory exists
// - Can be configured to skip stack validation using WithStackValidation(false)
//
// Returns specific, actionable errors (e.g., "directory for Atmos stacks does not exist")
// instead of generic errors, making it easier for users to diagnose configuration issues.
func ValidateAtmosConfig(opts ...ValidateOption) error {
	vCfg := &ValidateConfig{
		CheckStack: true, // Default value true to check the stack.
	}

	// Apply options.
	for _, opt := range opts {
		opt(vCfg)
	}

	// Use provided ConfigInfo to respect config selection flags (--config, etc.).
	atmosConfig, err := cfg.InitCliConfig(vCfg.ConfigInfo, false)
	if err != nil {
		return err
	}

	if vCfg.CheckStack {
		atmosConfigExists, err := u.IsDirectory(atmosConfig.StacksBaseAbsolutePath)
		if !atmosConfigExists || err != nil {
			// Return an error with context instead of printing and exiting.
			return errUtils.Build(errUtils.ErrStacksDirectoryDoesNotExist).
				WithHintf("Stacks directory not found:  \n%s", atmosConfig.StacksBaseAbsolutePath).
				WithContext("base_path", atmosConfig.BasePath).
				WithContext("stacks_base_path", atmosConfig.Stacks.BasePath).
				Err()
		}
	}

	return nil
}
