package shared

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
)

// BackendExecutionFlags returns flags for commands that generate backend files or run init.
func BackendExecutionFlags() *flags.FlagRegistry {
	registry := flags.NewFlagRegistry()
	registry.Register(&flags.StringFlag{
		Name:        "auto-generate-backend-file",
		Shorthand:   "",
		Default:     "",
		Description: "Override auto_generate_backend_file setting from atmos.yaml (true/false)",
		EnvVars:     []string{"ATMOS_AUTO_GENERATE_BACKEND_FILE"},
	})
	registry.Register(&flags.StringFlag{
		Name:        "init-run-reconfigure",
		Shorthand:   "",
		Default:     "",
		Description: "Override init_run_reconfigure setting from atmos.yaml (true/false)",
		EnvVars:     []string{"ATMOS_INIT_RUN_RECONFIGURE"},
	})
	return registry
}

// WithBackendExecutionFlags returns a flags.Option that adds backend execution flags.
func WithBackendExecutionFlags() flags.Option {
	return flags.WithFlagRegistry(BackendExecutionFlags())
}

// RegisterIdentityFlags adds identity and authentication related flags.
func RegisterIdentityFlags(registry *flags.FlagRegistry) {
	registry.Register(&flags.StringFlag{
		Name:        cfg.IdentityFlagName,
		Shorthand:   cfg.IdentityFlagShortName,
		Default:     "",
		Description: "Specify the identity to authenticate to before running Terraform commands. Use without value to interactively select.",
		EnvVars:     []string{"ATMOS_IDENTITY"},
		NoOptDefVal: cfg.IdentityFlagSelectValue,
	})
}
