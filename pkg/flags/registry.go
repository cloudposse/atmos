package flags

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
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
//   - stack (-s): Stack name
//   - identity (-i): Authentication identity (with NoOptDefVal for interactive selection)
//   - dry-run: Dry run mode
//
// Usage:
//
//	registry := flagparser.CommonFlags()
//	// Add command-specific flags
//	registry.Register(&flagparser.StringFlag{Name: "format", ...})
func CommonFlags() *FlagRegistry {
	defer perf.Track(nil, "flagparser.CommonFlags")()

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

	// Identity flag with NoOptDefVal for interactive selection
	registry.Register(&StringFlag{
		Name:        cfg.IdentityFlagName,
		Shorthand:   "i",
		Default:     "",
		Description: "Identity to use for authentication (use without value to select interactively)",
		Required:    false,
		NoOptDefVal: cfg.IdentityFlagSelectValue, // "__SELECT__"
		EnvVars:     []string{"ATMOS_IDENTITY", "IDENTITY"},
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
