package flags

import (
	"fmt"

	"github.com/spf13/cobra"
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
	defer perf.Track(nil, "flagparser.NewFlagRegistry")()

	return &FlagRegistry{
		flags: make(map[string]Flag),
	}
}

// Register adds a flag to the registry.
// Panics if a flag with the same name already exists to prevent duplicate registrations.
// This is a programming error that should be caught during development.
func (r *FlagRegistry) Register(flag Flag) {
	defer perf.Track(nil, "flagparser.FlagRegistry.Register")()

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
	defer perf.Track(nil, "flagparser.FlagRegistry.Get")()

	return r.flags[name]
}

// Has returns true if the registry contains a flag with the given name.
func (r *FlagRegistry) Has(name string) bool {
	defer perf.Track(nil, "flagparser.FlagRegistry.Has")()

	_, exists := r.flags[name]
	return exists
}

// All returns all registered flags.
// The returned slice is a copy and safe to modify.
func (r *FlagRegistry) All() []Flag {
	defer perf.Track(nil, "flagparser.FlagRegistry.All")()

	result := make([]Flag, 0, len(r.flags))
	for _, flag := range r.flags {
		result = append(result, flag)
	}
	return result
}

// Count returns the number of registered flags.
func (r *FlagRegistry) Count() int {
	defer perf.Track(nil, "flagparser.FlagRegistry.Count")()

	return len(r.flags)
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
//	registry := flagparser.CommonFlags()
//	// Add command-specific flags
//	registry.Register(&flagparser.StringFlag{Name: "format", ...})
func CommonFlags() *FlagRegistry {
	defer perf.Track(nil, "flagparser.CommonFlags")()

	// CommonFlags contains flags that are common across terraform/helmfile/packer commands
	// but are NOT global (not inherited from RootCmd).
	// Global flags (chdir, logs-level, base-path, etc.) are registered on RootCmd
	// as persistent flags and automatically inherited by all subcommands.
	registry := NewFlagRegistry()

	// Stack flag
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

// TerraformFlags returns a registry with flags specific to Terraform commands.
// Includes common flags plus Terraform-specific flags like:
//   - upload-status: Upload plan status to Atmos Pro
//   - skip-init: Skip terraform init
//   - from-plan: Apply from previously generated plan
func TerraformFlags() *FlagRegistry {
	defer perf.Track(nil, "flagparser.TerraformFlags")()

	registry := CommonFlags()

	// Upload status flag (optional bool - can be --upload-status or --upload-status=false)
	registry.Register(&BoolFlag{
		Name:        "upload-status",
		Shorthand:   "",
		Default:     false,
		Description: "Upload plan status to Atmos Pro",
		EnvVars:     []string{"ATMOS_UPLOAD_STATUS"},
	})

	// Skip init flag
	registry.Register(&BoolFlag{
		Name:        "skip-init",
		Shorthand:   "",
		Default:     false,
		Description: "Skip terraform init before running command",
		EnvVars:     []string{"ATMOS_SKIP_INIT"},
	})

	// From plan flag
	registry.Register(&StringFlag{
		Name:        "from-plan",
		Shorthand:   "",
		Default:     "",
		Description: "Apply from previously generated plan file",
		EnvVars:     []string{"ATMOS_FROM_PLAN"},
	})

	return registry
}

// HelmfileFlags returns a registry with flags specific to Helmfile commands.
func HelmfileFlags() *FlagRegistry {
	defer perf.Track(nil, "flagparser.HelmfileFlags")()

	registry := CommonFlags()

	// Helmfile-specific flags can be added here as needed

	return registry
}

// PackerFlags returns a registry with flags specific to Packer commands.
func PackerFlags() *FlagRegistry {
	defer perf.Track(nil, "flagparser.PackerFlags")()

	registry := CommonFlags()

	// Packer-specific flags can be added here as needed

	return registry
}

// Validate checks if all required flags are present and have valid values.
// Returns error if validation fails.
func (r *FlagRegistry) Validate(flagValues map[string]interface{}) error {
	defer perf.Track(nil, "flagparser.FlagRegistry.Validate")()

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
	defer perf.Track(nil, "flagparser.FlagRegistry.RegisterStringFlag")()

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
	defer perf.Track(nil, "flagparser.FlagRegistry.RegisterBoolFlag")()

	r.Register(&BoolFlag{
		Name:        name,
		Shorthand:   shorthand,
		Default:     defaultValue,
		Description: description,
	})
}

// RegisterIntFlag is a convenience method to register an integer flag.
func (r *FlagRegistry) RegisterIntFlag(name, shorthand string, defaultValue int, description string, required bool) {
	defer perf.Track(nil, "flagparser.FlagRegistry.RegisterIntFlag")()

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
	defer perf.Track(nil, "flagparser.FlagRegistry.RegisterFlags")()

	for _, flag := range r.flags {
		switch f := flag.(type) {
		case *StringFlag:
			cmd.Flags().StringP(f.Name, f.Shorthand, f.Default, f.Description)
			// Apply NoOptDefVal if set (for --flag syntax without value).
			if f.NoOptDefVal != "" {
				if err := cmd.Flags().SetAnnotation(f.Name, cobra.BashCompOneRequiredFlag, []string{"false"}); err == nil {
					cmd.Flags().Lookup(f.Name).NoOptDefVal = f.NoOptDefVal
				}
			}
		case *BoolFlag:
			cmd.Flags().BoolP(f.Name, f.Shorthand, f.Default, f.Description)
		case *IntFlag:
			cmd.Flags().IntP(f.Name, f.Shorthand, f.Default, f.Description)
		case *StringSliceFlag:
			cmd.Flags().StringSlice(f.Name, f.Default, f.Description)
		}
	}
}

// RegisterPersistentFlags registers all flags as persistent flags on the command.
// Persistent flags are inherited by subcommands.
func (r *FlagRegistry) RegisterPersistentFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.FlagRegistry.RegisterPersistentFlags")()

	for _, flag := range r.flags {
		switch f := flag.(type) {
		case *StringFlag:
			cmd.PersistentFlags().StringP(f.Name, f.Shorthand, f.Default, f.Description)
			// Apply NoOptDefVal if set (for --flag syntax without value).
			if f.NoOptDefVal != "" {
				if err := cmd.PersistentFlags().SetAnnotation(f.Name, cobra.BashCompOneRequiredFlag, []string{"false"}); err == nil {
					cmd.PersistentFlags().Lookup(f.Name).NoOptDefVal = f.NoOptDefVal
				}
			}
		case *BoolFlag:
			cmd.PersistentFlags().BoolP(f.Name, f.Shorthand, f.Default, f.Description)
		case *IntFlag:
			cmd.PersistentFlags().IntP(f.Name, f.Shorthand, f.Default, f.Description)
		case *StringSliceFlag:
			cmd.PersistentFlags().StringSlice(f.Name, f.Default, f.Description)
		}
	}
}

// PreprocessNoOptDefValArgs rewrites space-separated flag syntax to equals syntax
// for flags that have NoOptDefVal set.
//
// This maintains backward compatibility with user expectations while working within
// Cobra's documented behavior that NoOptDefVal requires equals syntax (pflag #134, #321, cobra #1962).
//
// Example:
//
//	Input:  ["--identity", "prod", "plan"]
//	Output: ["--identity=prod", "plan"]
//
// Only processes flags registered in this registry with non-empty NoOptDefVal.
// This includes both long form (--identity) and shorthand (-i).
func (r *FlagRegistry) PreprocessNoOptDefValArgs(args []string) []string {
	defer perf.Track(nil, "flagparser.FlagRegistry.PreprocessNoOptDefValArgs")()

	// Build a set of flag names (long and short) that have NoOptDefVal.
	noOptDefValFlags := make(map[string]bool)
	for _, flag := range r.flags {
		if flag.GetNoOptDefVal() != "" {
			noOptDefValFlags[flag.GetName()] = true
			if shorthand := flag.GetShorthand(); shorthand != "" {
				noOptDefValFlags[shorthand] = true
			}
		}
	}

	// If no flags have NoOptDefVal, return args unchanged.
	if len(noOptDefValFlags) == 0 {
		return args
	}

	// Preprocess args to rewrite space-separated syntax to equals syntax.
	result := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Check if this arg is a flag (starts with - or --).
		if !isFlagArg(arg) {
			result = append(result, arg)
			continue
		}

		// Check if flag already has equals syntax (--flag=value or -f=value).
		if hasSeparatedValue(arg) {
			result = append(result, arg)
			continue
		}

		// Extract flag name from --flag or -f.
		flagName := extractFlagName(arg)

		// Check if this flag has NoOptDefVal.
		if !noOptDefValFlags[flagName] {
			result = append(result, arg)
			continue
		}

		// Check if there's a next arg and it's not another flag.
		if i+1 < len(args) && !isFlagArg(args[i+1]) {
			// Combine current flag with next arg using equals syntax.
			combined := arg + "=" + args[i+1]
			result = append(result, combined)
			i++ // Skip the next arg (already consumed).
		} else {
			// No value follows, or next arg is another flag.
			// Keep flag as-is (will use NoOptDefVal).
			result = append(result, arg)
		}
	}

	return result
}

// isFlagArg returns true if the arg looks like a flag (starts with - or --).
func isFlagArg(arg string) bool {
	return len(arg) > 0 && arg[0] == '-'
}

// hasSeparatedValue returns true if the flag already has equals syntax (--flag=value or -f=value).
func hasSeparatedValue(arg string) bool {
	// Find first = after the leading dashes.
	for i := 0; i < len(arg); i++ {
		if arg[i] == '=' {
			return true
		}
		// Stop searching if we hit the end of the flag name.
		if i > 0 && arg[i] != '-' && arg[i-1] == '-' {
			break
		}
	}
	return false
}

// extractFlagName extracts the flag name from --flag or -f.
// Examples: --identity → identity, -i → i, --stack → stack.
func extractFlagName(arg string) string {
	// Strip leading dashes.
	name := arg
	for len(name) > 0 && name[0] == '-' {
		name = name[1:]
	}
	return name
}

// BindToViper binds all flags in this registry to a Viper instance.
// This is part of the Builder interface.
//
// Binding enables flag precedence: CLI > ENV > config > default.
// Each flag is bound to its environment variables if specified.
func (r *FlagRegistry) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.FlagRegistry.BindToViper")()

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
