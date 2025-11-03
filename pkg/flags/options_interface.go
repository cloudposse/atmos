package flags

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// CommandOptions is the base interface all strongly-typed interpreters implement.
// Each command type (Terraform, Helmfile, Auth, etc.) has its own interpreter struct
// that embeds GlobalFlags and provides command-specific fields.
//
// This enables strongly-typed access to flags instead of weak map-based access:
//
//	// ❌ Weak typing (old way with map access)
//	stack := atmosFlags["stack"].(string)  // runtime type assertion, can panic
//
//	// ✅ Strong typing (new way with interpreter)
//	stack := interpreter.Stack  // compile-time type safety
type CommandOptions interface {
	// GetGlobalFlags returns the global flags available to all commands.
	GetGlobalFlags() *GlobalFlags

	// GetPositionalArgs returns positional arguments (e.g., component name, subcommand).
	GetPositionalArgs() []string

	// GetPassThroughArgs returns arguments to pass to underlying tools (Terraform, Helmfile, etc.).
	GetPassThroughArgs() []string
}

// BaseOptions provides common implementation for CommandOptions interface.
// Command-specific interpreters should embed this (or just embed GlobalFlags directly).
type BaseOptions struct {
	GlobalFlags     // Embedded global flags.
	positionalArgs  []string
	passThroughArgs []string
}

// NewBaseOptions creates a new BaseOptions with the given arguments.
func NewBaseOptions(globalFlags GlobalFlags, positionalArgs, passThroughArgs []string) BaseOptions {
	defer perf.Track(nil, "flagparser.NewBaseOptions")()

	return BaseOptions{
		GlobalFlags:     globalFlags,
		positionalArgs:  positionalArgs,
		passThroughArgs: passThroughArgs,
	}
}

// GetPositionalArgs implements CommandOptions.
func (b *BaseOptions) GetPositionalArgs() []string {
	defer perf.Track(nil, "flagparser.BaseOptions.GetPositionalArgs")()

	return b.positionalArgs
}

// GetPassThroughArgs implements CommandOptions.
func (b *BaseOptions) GetPassThroughArgs() []string {
	defer perf.Track(nil, "flagparser.BaseOptions.GetPassThroughArgs")()

	return b.passThroughArgs
}
