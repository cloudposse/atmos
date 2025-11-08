package flags

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// Option is a functional option for configuring a FlagParser.
// This pattern allows for flexible, extensible configuration without
// breaking changes when new options are added.
//
// Usage:
//
//	parser := flagparser.NewStandardFlagParser(
//	    flagparser.WithStringFlag("stack", "s", "", "Stack name"),
//	    flagparser.WithBoolFlag("dry-run", "", false, "Dry run mode"),
//	)
type Option func(*parserConfig)

// parserConfig holds the configuration for a FlagParser.
// This is an internal type used by the Options pattern.
type parserConfig struct {
	registry    *FlagRegistry
	viperPrefix string // Prefix for Viper keys (optional)
}

// WithStringFlag adds a string flag to the parser configuration.
//
// Parameters:
//   - name: Long flag name (without --)
//   - shorthand: Short flag name (single character, without -)
//   - defaultValue: Default value if flag not provided
//   - description: Help text
//
// Usage:
//
//	WithStringFlag("stack", "s", "", "Stack name")
func WithStringFlag(name, shorthand, defaultValue, description string) Option {
	defer perf.Track(nil, "flags.WithStringFlag")()

	return func(cfg *parserConfig) {
		cfg.registry.Register(&StringFlag{
			Name:        name,
			Shorthand:   shorthand,
			Default:     defaultValue,
			Description: description,
		})
	}
}

// WithBoolFlag adds a boolean flag to the parser configuration.
func WithBoolFlag(name, shorthand string, defaultValue bool, description string) Option {
	defer perf.Track(nil, "flags.WithBoolFlag")()

	return func(cfg *parserConfig) {
		cfg.registry.Register(&BoolFlag{
			Name:        name,
			Shorthand:   shorthand,
			Default:     defaultValue,
			Description: description,
		})
	}
}

// WithIntFlag adds an integer flag to the parser configuration.
func WithIntFlag(name, shorthand string, defaultValue int, description string) Option {
	defer perf.Track(nil, "flags.WithIntFlag")()

	return func(cfg *parserConfig) {
		cfg.registry.Register(&IntFlag{
			Name:        name,
			Shorthand:   shorthand,
			Default:     defaultValue,
			Description: description,
		})
	}
}

// WithStringSliceFlag adds a string slice flag to the parser configuration.
//
// Parameters:
//   - name: Long flag name (without --)
//   - shorthand: Short flag name (single character, without -)
//   - defaultValue: Default value if flag not provided
//   - description: Help text
//
// Usage:
//
//	WithStringSliceFlag("components", "", []string{}, "Filter by components")
func WithStringSliceFlag(name, shorthand string, defaultValue []string, description string) Option {
	defer perf.Track(nil, "flags.WithStringSliceFlag")()

	return func(cfg *parserConfig) {
		cfg.registry.Register(&StringSliceFlag{
			Name:        name,
			Shorthand:   shorthand,
			Default:     defaultValue,
			Description: description,
		})
	}
}

// WithRequiredStringFlag adds a required string flag.
func WithRequiredStringFlag(name, shorthand, description string) Option {
	defer perf.Track(nil, "flags.WithRequiredStringFlag")()

	return func(cfg *parserConfig) {
		cfg.registry.Register(&StringFlag{
			Name:        name,
			Shorthand:   shorthand,
			Default:     "",
			Description: description,
			Required:    true,
		})
	}
}

// WithIdentityFlag adds the identity flag with NoOptDefVal support.
// This enables the pattern: --identity (interactive), --identity value (explicit).
//
// The identity flag:
//   - Supports --identity=value and --identity value forms
//   - Uses NoOptDefVal for interactive selection when flag is alone
//   - Binds to ATMOS_IDENTITY and IDENTITY env vars
//   - Respects precedence: flag > env > config > default
func WithIdentityFlag() Option {
	defer perf.Track(nil, "flags.WithIdentityFlag")()

	return func(cfg *parserConfig) {
		flag := GlobalFlagsRegistry().Get("identity")
		if flag != nil {
			cfg.registry.Register(flag)
		}
	}
}

// WithStackFlag adds the stack flag (-s).
func WithStackFlag() Option {
	defer perf.Track(nil, "flags.WithStackFlag")()

	return func(cfg *parserConfig) {
		flag := CommonFlags().Get("stack")
		if flag != nil {
			cfg.registry.Register(flag)
		}
	}
}

// WithDryRunFlag adds the dry-run flag.
func WithDryRunFlag() Option {
	defer perf.Track(nil, "flags.WithDryRunFlag")()

	return func(cfg *parserConfig) {
		flag := CommonFlags().Get("dry-run")
		if flag != nil {
			cfg.registry.Register(flag)
		}
	}
}

// WithCommonFlags adds all common Atmos flags (stack, identity, dry-run).
func WithCommonFlags() Option {
	defer perf.Track(nil, "flags.WithCommonFlags")()

	return func(cfg *parserConfig) {
		// CommonFlags now includes global.Flags + common flags (stack, dry-run).
		// First add global flags.
		for _, flag := range GlobalFlagsRegistry().All() {
			cfg.registry.Register(flag)
		}
		// Then add common flags (stack, dry-run).
		for _, flag := range CommonFlags().All() {
			// Skip if already registered (e.g., identity flag from global.Flags).
			if !cfg.registry.Has(flag.GetName()) {
				cfg.registry.Register(flag)
			}
		}
	}
}

// WithTerraformFlags adds all Terraform-specific flags.
// Global flags (identity, chdir, etc.) are inherited from RootCmd persistent flags, not registered here.
func WithTerraformFlags() Option {
	defer perf.Track(nil, "flags.WithTerraformFlags")()

	return func(cfg *parserConfig) {
		for _, flag := range TerraformFlags().All() {
			cfg.registry.Register(flag)
		}
	}
}

// WithHelmfileFlags adds all Helmfile-specific flags.
func WithHelmfileFlags() Option {
	defer perf.Track(nil, "flags.WithHelmfileFlags")()

	return func(cfg *parserConfig) {
		for _, flag := range HelmfileFlags().All() {
			cfg.registry.Register(flag)
		}
	}
}

// WithPackerFlags adds all Packer-specific flags.
func WithPackerFlags() Option {
	defer perf.Track(nil, "flags.WithPackerFlags")()

	return func(cfg *parserConfig) {
		for _, flag := range PackerFlags().All() {
			cfg.registry.Register(flag)
		}
	}
}

// WithEnvVars adds environment variable bindings to a flag.
// Must be called after the flag is added.
//
// Usage:
//
//	WithStringFlag("format", "f", "yaml", "Output format"),
//	WithEnvVars("format", "ATMOS_FORMAT", "FORMAT"),
func WithEnvVars(flagName string, envVars ...string) Option {
	defer perf.Track(nil, "flags.WithEnvVars")()

	return func(cfg *parserConfig) {
		flag := cfg.registry.Get(flagName)
		if flag == nil {
			return
		}

		// Update the flag with env vars based on type.
		// Note: No need to re-register - the flag is already in the registry.
		// We're just updating its EnvVars field in place.
		switch f := flag.(type) {
		case *StringFlag:
			f.EnvVars = envVars
		case *BoolFlag:
			f.EnvVars = envVars
		case *IntFlag:
			f.EnvVars = envVars
		case *StringSliceFlag:
			f.EnvVars = envVars
		}
	}
}

// WithNoOptDefVal sets the NoOptDefVal for a string flag.
// This enables the flag to have a special value when used without an argument.
//
// Example:
//
//	WithStringFlag("identity", "i", "", "Identity name"),
//	WithNoOptDefVal("identity", "__SELECT__"),  // --identity alone = "__SELECT__"
func WithNoOptDefVal(flagName, value string) Option {
	defer perf.Track(nil, "flags.WithNoOptDefVal")()

	return func(cfg *parserConfig) {
		flag := cfg.registry.Get(flagName)
		if strFlag, ok := flag.(*StringFlag); ok {
			strFlag.NoOptDefVal = value
			// Note: No need to re-register - we're just updating the field in place.
		}
	}
}

// WithValidValues sets the list of valid values for a string flag.
// During parsing, the flag value will be validated against this list.
//
// Example:
//
//	WithStringFlag("format", "f", "yaml", "Output format"),
//	WithValidValues("format", "json", "yaml", "table"),
func WithValidValues(flagName string, validValues ...string) Option {
	defer perf.Track(nil, "flags.WithValidValues")()

	return func(cfg *parserConfig) {
		flag := cfg.registry.Get(flagName)
		if strFlag, ok := flag.(*StringFlag); ok {
			strFlag.ValidValues = validValues
			// Note: No need to re-register - we're just updating the field in place.
		}
	}
}

// WithViperPrefix sets a prefix for all Viper keys.
// This is useful for namespacing flags in config files.
//
// Example:
//
//	WithViperPrefix("terraform")  // flags stored as terraform.stack, terraform.identity, etc.
func WithViperPrefix(prefix string) Option {
	defer perf.Track(nil, "flags.WithViperPrefix")()

	return func(cfg *parserConfig) {
		cfg.viperPrefix = prefix
	}
}

// WithRegistry uses a pre-configured FlagRegistry instead of building one from options.
// This is useful when you want full control over flag configuration.
//
// Usage:
//
//	registry := flagparser.NewFlagRegistry()
//	registry.Register(&flagparser.StringFlag{...})
//	parser := flagparser.NewStandardFlagParser(flagparser.WithRegistry(registry))
func WithRegistry(registry *FlagRegistry) Option {
	defer perf.Track(nil, "flags.WithRegistry")()

	return func(cfg *parserConfig) {
		cfg.registry = registry
	}
}
