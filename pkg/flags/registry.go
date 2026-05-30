package flags

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// FlagRegistry stores reusable flag definitions.
// This allows common flags (stack, identity, dry-run) to be defined once
// and reused across multiple commands.
type FlagRegistry struct {
	flags map[string]Flag
}

// NewFlagRegistry creates a new flag registry.
func NewFlagRegistry() *FlagRegistry {
	defer perf.Track(nil, "flags.NewFlagRegistry")()

	return &FlagRegistry{
		flags: make(map[string]Flag),
	}
}

// Register adds a flag to the registry.
// Panics if a flag with the same name already exists to prevent duplicate registrations.
// This is a programming error that should be caught during development.
func (r *FlagRegistry) Register(flag Flag) {
	defer perf.Track(nil, "flags.FlagRegistry.Register")()

	flagName := flag.GetName()
	if r.Has(flagName) {
		panic(fmt.Errorf("%w: flag '%s' is already registered; this is a programming error - "+
			"flags should only be registered once; check for duplicate flag definitions in "+
			"registry functions (CommonFlags, TerraformFlags, etc.) or duplicate manual registrations "+
			"in command init() functions", errUtils.ErrDuplicateFlagRegistration, flagName))
	}
	r.flags[flagName] = flag
}

// Get retrieves a flag by name.
// Returns nil if flag not found.
func (r *FlagRegistry) Get(name string) Flag {
	defer perf.Track(nil, "flags.FlagRegistry.Get")()

	return r.flags[name]
}

// Has returns true if the registry contains a flag with the given name.
func (r *FlagRegistry) Has(name string) bool {
	defer perf.Track(nil, "flags.FlagRegistry.Has")()

	_, exists := r.flags[name]
	return exists
}

// All returns all registered flags.
// The returned slice is a copy and safe to modify.
func (r *FlagRegistry) All() []Flag {
	defer perf.Track(nil, "flags.FlagRegistry.All")()

	result := make([]Flag, 0, len(r.flags))
	for _, flag := range r.flags {
		result = append(result, flag)
	}
	return result
}

// Count returns the number of registered flags.
func (r *FlagRegistry) Count() int {
	defer perf.Track(nil, "flags.FlagRegistry.Count")()

	return len(r.flags)
}

// SetCompletionFunc sets a custom completion function for a flag.
// This is used to set completion functions after flag registration to avoid import cycles.
// For example, cmd/terraform can set the stack completion function without pkg/flags needing
// to import internal/exec.
func (r *FlagRegistry) SetCompletionFunc(name string, fn func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)) {
	defer perf.Track(nil, "flags.FlagRegistry.SetCompletionFunc")()

	flag := r.flags[name]
	if flag == nil {
		return
	}

	// Only StringFlag supports completion functions currently.
	if stringFlag, ok := flag.(*StringFlag); ok {
		stringFlag.CompletionFunc = fn
	}
}

// CommonFlags returns a registry pre-populated with common Atmos flags.
// This includes:
//   - All global flags from GlobalFlagsRegistry() (logs-level, chdir, base-path, identity, etc.)
//   - stack (-s): Stack name
//   - dry-run: Dry run mode
//
// Note: identity is already in GlobalFlagsRegistry(), so it's not duplicated here.
//
// Usage:
//
//	registry := flags.CommonFlags()
//	// Add command-specific flags
//	registry.Register(&flags.StringFlag{Name: "format", ...})
func CommonFlags() *FlagRegistry {
	defer perf.Track(nil, "flags.CommonFlags")()

	// CommonFlags contains flags that are common across terraform/helmfile/packer commands
	// but are NOT global (not inherited from RootCmd).
	// Global flags (chdir, logs-level, base-path, etc.) are registered on RootCmd
	// as persistent flags and automatically inherited by all subcommands.
	registry := NewFlagRegistry()

	// Stack flag
	// Note: CompletionFunc is set by cmd/terraform package to avoid import cycle.
	registry.Register(&StringFlag{
		Name:        "stack",
		Shorthand:   "s",
		Default:     "",
		Description: "Stack name",
		Required:    false,
		EnvVars:     []string{"ATMOS_STACK"},
	})

	// Dry run flag
	registry.Register(&BoolFlag{
		Name:        "dry-run",
		Shorthand:   "",
		Default:     false,
		Description: "Perform dry run without making actual changes",
		EnvVars:     []string{"ATMOS_DRY_RUN"},
	})

	return registry
}

// HelmfileFlags returns a registry with flags specific to Helmfile commands.
func HelmfileFlags() *FlagRegistry {
	defer perf.Track(nil, "flags.HelmfileFlags")()

	registry := CommonFlags()

	// Helmfile-specific flags can be added here as needed

	return registry
}

// PackerFlags returns a registry with flags specific to Packer commands.
func PackerFlags() *FlagRegistry {
	defer perf.Track(nil, "flags.PackerFlags")()

	registry := CommonFlags()

	// Packer-specific flags can be added here as needed

	return registry
}

// Validate checks if all required flags are present and have valid values.
// Returns error if validation fails.
func (r *FlagRegistry) Validate(flagValues map[string]interface{}) error {
	defer perf.Track(nil, "flags.FlagRegistry.Validate")()

	for _, flag := range r.flags {
		if !flag.IsRequired() {
			continue
		}

		value, exists := flagValues[flag.GetName()]
		if !exists {
			return fmt.Errorf("%w: --%s", errUtils.ErrRequiredFlagNotProvided, flag.GetName())
		}

		// Check for empty values on required string flags
		if strFlag, ok := flag.(*StringFlag); ok {
			if strVal, ok := value.(string); ok && strVal == "" {
				return fmt.Errorf("%w: --%s", errUtils.ErrRequiredFlagEmpty, strFlag.Name)
			}
		}
	}

	return nil
}

// RegisterStringFlag is a convenience method to register a string flag.
func (r *FlagRegistry) RegisterStringFlag(name, shorthand, defaultValue, description string, required bool) {
	defer perf.Track(nil, "flags.FlagRegistry.RegisterStringFlag")()

	r.Register(&StringFlag{
		Name:        name,
		Shorthand:   shorthand,
		Default:     defaultValue,
		Description: description,
		Required:    required,
	})
}

// RegisterBoolFlag is a convenience method to register a boolean flag.
func (r *FlagRegistry) RegisterBoolFlag(name, shorthand string, defaultValue bool, description string) {
	defer perf.Track(nil, "flags.FlagRegistry.RegisterBoolFlag")()

	r.Register(&BoolFlag{
		Name:        name,
		Shorthand:   shorthand,
		Default:     defaultValue,
		Description: description,
	})
}

// RegisterIntFlag is a convenience method to register an integer flag.
func (r *FlagRegistry) RegisterIntFlag(name, shorthand string, defaultValue int, description string, required bool) {
	defer perf.Track(nil, "flags.FlagRegistry.RegisterIntFlag")()

	r.Register(&IntFlag{
		Name:        name,
		Shorthand:   shorthand,
		Default:     defaultValue,
		Description: description,
		Required:    required,
	})
}

// RegisterFlags registers all flags in this registry with a Cobra command.
// This is part of the Builder interface.
//
// Each flag type is registered with its appropriate Cobra method:
//   - StringFlag → cmd.Flags().StringP()
//   - BoolFlag → cmd.Flags().BoolP()
//   - IntFlag → cmd.Flags().IntP()
//   - StringSliceFlag → cmd.Flags().StringSlice()
func (r *FlagRegistry) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flags.FlagRegistry.RegisterFlags")()

	for _, flag := range r.flags {
		r.registerFlagToSet(cmd.Flags(), flag)
	}
}

// registerFlagToSet is a helper that registers a single flag to a pflag.FlagSet.
// This eliminates duplication between RegisterFlags and RegisterPersistentFlags.
func (r *FlagRegistry) registerFlagToSet(flagSet *pflag.FlagSet, flag Flag) {
	switch f := flag.(type) {
	case *StringFlag:
		flagSet.StringP(f.Name, f.Shorthand, f.Default, f.Description)
		// Apply NoOptDefVal if set (for --flag syntax without value).
		if f.NoOptDefVal != "" {
			if err := flagSet.SetAnnotation(f.Name, cobra.BashCompOneRequiredFlag, []string{"false"}); err == nil {
				flagSet.Lookup(f.Name).NoOptDefVal = f.NoOptDefVal
			}
		}
	case *BoolFlag:
		flagSet.BoolP(f.Name, f.Shorthand, f.Default, f.Description)
	case *IntFlag:
		flagSet.IntP(f.Name, f.Shorthand, f.Default, f.Description)
	case *StringSliceFlag:
		flagSet.StringSlice(f.Name, f.Default, f.Description)
	}
}

// RegisterPersistentFlags registers all flags as persistent flags on the command.
// Persistent flags are inherited by subcommands.
func (r *FlagRegistry) RegisterPersistentFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flags.FlagRegistry.RegisterPersistentFlags")()

	for _, flag := range r.flags {
		r.registerFlagToSet(cmd.PersistentFlags(), flag)
	}
}

// BindToViper binds all flags in this registry to a Viper instance.
// This is part of the Builder interface.
//
// Binding enables flag precedence: CLI > ENV > config > default.
// Each flag is bound to its environment variables if specified.
func (r *FlagRegistry) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flags.FlagRegistry.BindToViper")()

	for _, flag := range r.flags {
		// Bind environment variables if specified
		envVars := flag.GetEnvVars()
		if len(envVars) > 0 {
			// Create variadic args: (key, env_var1, env_var2, ...)
			args := make([]string, 0, len(envVars)+1)
			args = append(args, flag.GetName())
			args = append(args, envVars...)

			if err := v.BindEnv(args...); err != nil {
				return fmt.Errorf("failed to bind env vars for flag %s: %w", flag.GetName(), err)
			}
		}
	}

	return nil
}
