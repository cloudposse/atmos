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
	registerInitOverrideFlags(registry)
	return registry
}

// WithBackendExecutionFlags returns a flags.Option that adds backend execution flags.
func WithBackendExecutionFlags() flags.Option {
	return flags.WithFlagRegistry(BackendExecutionFlags())
}

// registerInitOverrideFlags adds the tri-state components.terraform.init.mode/reconfigure/upgrade
// override flags. Shared by BackendExecutionFlags (plan/apply/deploy/destroy/refresh/init/
// workspace/migrate/test) and InitOverrideFlags (output/shell, which don't pull in the rest of
// BackendExecutionFlags) so the flag definitions never drift between the two call sites.
func registerInitOverrideFlags(registry *flags.FlagRegistry) {
	registry.Register(&flags.StringFlag{
		Name:        "init-mode",
		Shorthand:   "",
		Default:     "",
		Description: "Override init.mode from atmos.yaml (auto, always, never)",
		EnvVars:     []string{"ATMOS_INIT_MODE"},
	})
	registry.Register(&flags.StringFlag{
		Name:        "init-reconfigure",
		Shorthand:   "",
		Default:     "",
		Description: "Override init.reconfigure from atmos.yaml (auto, always, never)",
		EnvVars:     []string{"ATMOS_INIT_RECONFIGURE"},
	})
	registry.Register(&flags.StringFlag{
		Name:        "init-upgrade",
		Shorthand:   "",
		Default:     "",
		Description: "Override init.upgrade from atmos.yaml (auto, always, never)",
		EnvVars:     []string{"ATMOS_INIT_UPGRADE"},
	})
}

// InitOverrideFlags returns a flag registry containing just the tri-state init.mode/
// init.reconfigure/init.upgrade override flags, for commands like `output` and `shell` that
// register their own flag set instead of pulling in the full BackendExecutionFlags registry.
func InitOverrideFlags() *flags.FlagRegistry {
	registry := flags.NewFlagRegistry()
	registerInitOverrideFlags(registry)
	return registry
}

// WithInitOverrideFlags returns a flags.Option that adds the tri-state init override flags.
func WithInitOverrideFlags() flags.Option {
	return flags.WithFlagRegistry(InitOverrideFlags())
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
