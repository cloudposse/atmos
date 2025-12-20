package terraform

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// TerraformFlags returns a registry with flags specific to Terraform commands.
// Includes common flags plus Terraform-specific flags.
func TerraformFlags() *flags.FlagRegistry {
	defer perf.Track(nil, "terraform.TerraformFlags")()

	registry := flags.CommonFlags()
	registerIdentityFlags(registry)
	registerExecutionFlags(registry)
	registerProcessingFlags(registry)
	registerFilterFlags(registry)
	return registry
}

// registerIdentityFlags adds identity and authentication related flags.
func registerIdentityFlags(registry *flags.FlagRegistry) {
	registry.Register(&flags.StringFlag{
		Name:        "identity",
		Shorthand:   "i",
		Default:     "",
		Description: "Specify the identity to authenticate to before running Terraform commands. Use without value to interactively select.",
		EnvVars:     []string{"ATMOS_IDENTITY", "IDENTITY"},
		NoOptDefVal: "__SELECT__",
	})
}

// registerExecutionFlags adds flags related to terraform execution behavior.
func registerExecutionFlags(registry *flags.FlagRegistry) {
	registry.Register(&flags.BoolFlag{
		Name:        "upload-status",
		Shorthand:   "",
		Default:     false,
		Description: "Upload plan status to Atmos Pro",
		EnvVars:     []string{"ATMOS_UPLOAD_STATUS"},
	})
	registry.Register(&flags.BoolFlag{
		Name:        "skip-init",
		Shorthand:   "",
		Default:     false,
		Description: "Skip terraform init before running command",
		EnvVars:     []string{"ATMOS_SKIP_INIT"},
	})
	// Note: from-plan flag is defined in apply.go and deploy.go with NoOptDefVal
	// to support both --from-plan (boolean-like) and --from-plan=<path> usage.
	registry.Register(&flags.BoolFlag{
		Name:        "init-pass-vars",
		Shorthand:   "",
		Default:     false,
		Description: "Pass the generated varfile to terraform init using --var-file flag (OpenTofu feature)",
		EnvVars:     []string{"ATMOS_INIT_PASS_VARS"},
	})
	registry.Register(&flags.StringFlag{
		Name:        "append-user-agent",
		Shorthand:   "",
		Default:     "",
		Description: "Customize User-Agent string in Terraform provider requests (sets TF_APPEND_USER_AGENT)",
		EnvVars:     []string{"ATMOS_APPEND_USER_AGENT"},
	})
}

// BackendExecutionFlags returns flags for commands that generate backend files or run init.
// These flags are used by: init, workspace, plan, apply, deploy.
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

// registerProcessingFlags adds flags for template and function processing.
func registerProcessingFlags(registry *flags.FlagRegistry) {
	registry.Register(&flags.BoolFlag{
		Name:        "process-templates",
		Shorthand:   "",
		Default:     true,
		Description: "Enable/disable Go template processing in Atmos stack manifests",
		EnvVars:     []string{"ATMOS_PROCESS_TEMPLATES"},
	})
	registry.Register(&flags.BoolFlag{
		Name:        "process-functions",
		Shorthand:   "",
		Default:     true,
		Description: "Enable/disable YAML functions processing in Atmos stack manifests",
		EnvVars:     []string{"ATMOS_PROCESS_FUNCTIONS"},
	})
	registry.Register(&flags.StringSliceFlag{
		Name:        "skip",
		Shorthand:   "",
		Default:     nil,
		Description: "Skip executing specific YAML functions in the Atmos stack manifests",
		EnvVars:     []string{"ATMOS_SKIP"},
	})
}

// registerFilterFlags adds flags for filtering components.
func registerFilterFlags(registry *flags.FlagRegistry) {
	registry.Register(&flags.StringFlag{
		Name:        "query",
		Shorthand:   "q",
		Default:     "",
		Description: "Execute atmos terraform command on components filtered by a YQ expression",
		EnvVars:     []string{"ATMOS_QUERY"},
	})
	registry.Register(&flags.StringSliceFlag{
		Name:        "components",
		Shorthand:   "",
		Default:     nil,
		Description: "Filter by specific components",
		EnvVars:     []string{"ATMOS_COMPONENTS"},
	})
}

// TerraformAffectedFlags returns a registry with flags for affected component detection.
// These flags are used by commands that support --affected mode.
func TerraformAffectedFlags() *flags.FlagRegistry {
	defer perf.Track(nil, "terraform.TerraformAffectedFlags")()

	registry := flags.NewFlagRegistry()
	registerGitReferenceFlags(registry)
	registerSSHFlags(registry)
	registerAffectedBehaviorFlags(registry)
	return registry
}

// registerGitReferenceFlags adds flags for Git reference specification.
func registerGitReferenceFlags(registry *flags.FlagRegistry) {
	registry.Register(&flags.StringFlag{
		Name:        "repo-path",
		Shorthand:   "",
		Default:     "",
		Description: "Filesystem path to the already cloned target repository with which to compare the current branch",
		EnvVars:     []string{"ATMOS_REPO_PATH"},
	})
	registry.Register(&flags.StringFlag{
		Name:        "ref",
		Shorthand:   "",
		Default:     "",
		Description: "Git reference with which to compare the current branch",
		EnvVars:     []string{"ATMOS_REF"},
	})
	registry.Register(&flags.StringFlag{
		Name:        "sha",
		Shorthand:   "",
		Default:     "",
		Description: "Git commit SHA with which to compare the current branch",
		EnvVars:     []string{"ATMOS_SHA"},
	})
}

// registerSSHFlags adds flags for SSH key configuration.
func registerSSHFlags(registry *flags.FlagRegistry) {
	registry.Register(&flags.StringFlag{
		Name:        "ssh-key",
		Shorthand:   "",
		Default:     "",
		Description: "Path to PEM-encoded private key to clone private repos using SSH",
		EnvVars:     []string{"ATMOS_SSH_KEY"},
	})
	registry.Register(&flags.StringFlag{
		Name:        "ssh-key-password",
		Shorthand:   "",
		Default:     "",
		Description: "Encryption password for the PEM-encoded private key if the key contains a password-encrypted PEM block",
		EnvVars:     []string{"ATMOS_SSH_KEY_PASSWORD"},
	})
}

// registerAffectedBehaviorFlags adds flags for affected command behavior.
func registerAffectedBehaviorFlags(registry *flags.FlagRegistry) {
	registry.Register(&flags.BoolFlag{
		Name:        "include-dependents",
		Shorthand:   "",
		Default:     false,
		Description: "For each affected component, detect the dependent components and process them in the dependency order",
		EnvVars:     []string{"ATMOS_INCLUDE_DEPENDENTS"},
	})
	registry.Register(&flags.BoolFlag{
		Name:        "clone-target-ref",
		Shorthand:   "",
		Default:     false,
		Description: "Clone the target reference with which to compare the current branch",
		EnvVars:     []string{"ATMOS_CLONE_TARGET_REF"},
	})
}

// WithTerraformFlags returns a flags.Option that adds all Terraform-specific flags.
func WithTerraformFlags() flags.Option {
	defer perf.Track(nil, "terraform.WithTerraformFlags")()

	return flags.WithFlagRegistry(TerraformFlags())
}

// WithTerraformAffectedFlags returns a flags.Option that adds affected component detection flags.
func WithTerraformAffectedFlags() flags.Option {
	defer perf.Track(nil, "terraform.WithTerraformAffectedFlags")()

	return flags.WithFlagRegistry(TerraformAffectedFlags())
}
