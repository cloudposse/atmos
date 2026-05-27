package rain

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

func RainFlags() *flags.FlagRegistry {
	defer perf.Track(nil, "rain.RainFlags")()

	registry := flags.CommonFlags()
	registerIdentityFlags(registry)
	registerProcessingFlags(registry)
	registerFilterFlags(registry)
	registerRainSpecificFlags(registry)
	registerGitFlags(registry)
	return registry
}

func registerIdentityFlags(registry *flags.FlagRegistry) {
	registry.Register(&flags.StringFlag{
		Name:        cfg.IdentityFlagName,
		Shorthand:   cfg.IdentityFlagShortName,
		Default:     "",
		Description: "Specify the identity to authenticate before running Rain commands. Use without value to interactively select.",
		EnvVars:     []string{"ATMOS_IDENTITY"},
		NoOptDefVal: cfg.IdentityFlagSelectValue,
	})
}

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

func registerFilterFlags(registry *flags.FlagRegistry) {
	registry.Register(&flags.BoolFlag{
		Name:        "all",
		Shorthand:   "",
		Default:     false,
		Description: "Run the Rain command for all matching Rain components",
		EnvVars:     []string{"ATMOS_ALL"},
	})
	registry.Register(&flags.BoolFlag{
		Name:        "affected",
		Shorthand:   "",
		Default:     false,
		Description: "Run the Rain command for Git-affected Rain components",
		EnvVars:     []string{"ATMOS_AFFECTED"},
	})
	registry.Register(&flags.StringFlag{
		Name:        "query",
		Shorthand:   "q",
		Default:     "",
		Description: "Run the Rain command on components filtered by a YQ expression",
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

func registerRainSpecificFlags(registry *flags.FlagRegistry) {
	registry.Register(&flags.StringFlag{
		Name:        "rain-command",
		Shorthand:   "",
		Default:     "",
		Description: "Rain executable to run",
		EnvVars:     []string{"ATMOS_RAIN_COMMAND"},
	})
	registry.Register(&flags.StringFlag{
		Name:        "rain-dir",
		Shorthand:   "",
		Default:     "",
		Description: "Rain components directory",
		EnvVars:     []string{"ATMOS_COMPONENTS_RAIN_BASE_PATH"},
	})
}

func registerGitFlags(registry *flags.FlagRegistry) {
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
	registry.Register(&flags.BoolFlag{
		Name:        "clone-target-ref",
		Shorthand:   "",
		Default:     false,
		Description: "Clone the target reference with which to compare the current branch",
		EnvVars:     []string{"ATMOS_CLONE_TARGET_REF"},
	})
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
		Description: "Encryption password for the PEM-encoded private key",
		EnvVars:     []string{"ATMOS_SSH_KEY_PASSWORD"},
	})
	registry.Register(&flags.BoolFlag{
		Name:        "include-dependents",
		Shorthand:   "",
		Default:     false,
		Description: "Include dependent components",
		EnvVars:     []string{"ATMOS_INCLUDE_DEPENDENTS"},
	})
}

func WithRainFlags() flags.Option {
	defer perf.Track(nil, "rain.WithRainFlags")()

	return flags.WithFlagRegistry(RainFlags())
}
