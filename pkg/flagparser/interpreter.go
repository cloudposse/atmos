package flagparser

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// CommandInterpreter is the base interface all strongly-typed interpreters implement.
// Each command type (Terraform, Helmfile, Auth, etc.) has its own interpreter struct
// that embeds GlobalFlags and provides command-specific fields.
//
// This enables strongly-typed access to flags instead of weak string-based access:
//
//	// ❌ Weak typing (old way)
//	stack := parsedConfig.AtmosFlags["stack"].(string)
//
//	// ✅ Strong typing (new way)
//	stack := interpreter.Stack
type CommandInterpreter interface {
	// GetGlobalFlags returns the global flags available to all commands.
	GetGlobalFlags() *GlobalFlags

	// GetPositionalArgs returns positional arguments (e.g., component name, subcommand).
	GetPositionalArgs() []string

	// GetPassThroughArgs returns arguments to pass to underlying tools (Terraform, Helmfile, etc.).
	GetPassThroughArgs() []string
}

// BaseInterpreter provides common implementation for CommandInterpreter interface.
// Command-specific interpreters should embed this (or just embed GlobalFlags directly).
type BaseInterpreter struct {
	GlobalFlags     // Embedded global flags.
	positionalArgs  []string
	passThroughArgs []string
}

// NewBaseInterpreter creates a new BaseInterpreter with the given arguments.
func NewBaseInterpreter(globalFlags GlobalFlags, positionalArgs, passThroughArgs []string) BaseInterpreter {
	defer perf.Track(nil, "flagparser.NewBaseInterpreter")()

	return BaseInterpreter{
		GlobalFlags:     globalFlags,
		positionalArgs:  positionalArgs,
		passThroughArgs: passThroughArgs,
	}
}

// GetPositionalArgs implements CommandInterpreter.
func (b *BaseInterpreter) GetPositionalArgs() []string {
	defer perf.Track(nil, "flagparser.BaseInterpreter.GetPositionalArgs")()

	return b.positionalArgs
}

// GetPassThroughArgs implements CommandInterpreter.
func (b *BaseInterpreter) GetPassThroughArgs() []string {
	defer perf.Track(nil, "flagparser.BaseInterpreter.GetPassThroughArgs")()

	return b.passThroughArgs
}
