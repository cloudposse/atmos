package flagparser

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
	defer perf.Track(nil, "flagparser.WithStringFlag")()

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
	defer perf.Track(nil, "flagparser.WithBoolFlag")()

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
	defer perf.Track(nil, "flagparser.WithIntFlag")()

	return func(cfg *parserConfig) {
		cfg.registry.Register(&IntFlag{
			Name:        name,
			Shorthand:   shorthand,
			Default:     defaultValue,
			Description: description,
		})
	}
}

// WithRequiredStringFlag adds a required string flag.
func WithRequiredStringFlag(name, shorthand, description string) Option {
	defer perf.Track(nil, "flagparser.WithRequiredStringFlag")()

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
	defer perf.Track(nil, "flagparser.WithIdentityFlag")()

	return func(cfg *parserConfig) {
		flag := CommonFlags().Get("identity")
		if flag != nil {
			cfg.registry.Register(flag)
		}
	}
}

// WithStackFlag adds the stack flag (-s).
func WithStackFlag() Option {
	defer perf.Track(nil, "flagparser.WithStackFlag")()

	return func(cfg *parserConfig) {
		flag := CommonFlags().Get("stack")
		if flag != nil {
			cfg.registry.Register(flag)
		}
	}
}

// WithDryRunFlag adds the dry-run flag.
func WithDryRunFlag() Option {
	defer perf.Track(nil, "flagparser.WithDryRunFlag")()

	return func(cfg *parserConfig) {
		flag := CommonFlags().Get("dry-run")
		if flag != nil {
			cfg.registry.Register(flag)
		}
	}
}

// WithCommonFlags adds all common Atmos flags (stack, identity, dry-run).
func WithCommonFlags() Option {
	defer perf.Track(nil, "flagparser.WithCommonFlags")()

	return func(cfg *parserConfig) {
		for _, flag := range CommonFlags().All() {
			cfg.registry.Register(flag)
		}
	}
}

// WithTerraformFlags adds all Terraform-specific flags.
func WithTerraformFlags() Option {
	defer perf.Track(nil, "flagparser.WithTerraformFlags")()

	return func(cfg *parserConfig) {
		for _, flag := range TerraformFlags().All() {
			cfg.registry.Register(flag)
		}
	}
}

// WithHelmfileFlags adds all Helmfile-specific flags.
func WithHelmfileFlags() Option {
	defer perf.Track(nil, "flagparser.WithHelmfileFlags")()

	return func(cfg *parserConfig) {
		for _, flag := range HelmfileFlags().All() {
			cfg.registry.Register(flag)
		}
	}
}

// WithPackerFlags adds all Packer-specific flags.
func WithPackerFlags() Option {
	defer perf.Track(nil, "flagparser.WithPackerFlags")()

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
	defer perf.Track(nil, "flagparser.WithEnvVars")()

	return func(cfg *parserConfig) {
		flag := cfg.registry.Get(flagName)
		if flag == nil {
			return
		}

		// Update the flag with env vars based on type
		switch f := flag.(type) {
		case *StringFlag:
			f.EnvVars = envVars
			cfg.registry.Register(f)
		case *BoolFlag:
			f.EnvVars = envVars
			cfg.registry.Register(f)
		case *IntFlag:
			f.EnvVars = envVars
			cfg.registry.Register(f)
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
	defer perf.Track(nil, "flagparser.WithNoOptDefVal")()

	return func(cfg *parserConfig) {
		flag := cfg.registry.Get(flagName)
		if strFlag, ok := flag.(*StringFlag); ok {
			strFlag.NoOptDefVal = value
			cfg.registry.Register(strFlag)
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
	defer perf.Track(nil, "flagparser.WithViperPrefix")()

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
	defer perf.Track(nil, "flagparser.WithRegistry")()

	return func(cfg *parserConfig) {
		cfg.registry = registry
	}
}
