package internal

import (
	"fmt"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// Context keys for passing values through cobra command context.
type contextKey string

// IoContextKey is the key for storing io.Context in cobra command context.
const IoContextKey contextKey = "ioContext"

// registry is the global command registry instance.
var registry = &CommandRegistry{
	providers: make(map[string]CommandProvider),
}

// CommandRegistry manages built-in command registration.
//
// This registry is for BUILT-IN commands only. Custom commands from atmos.yaml
// are processed separately via processCustomCommands() in cmd/cmd_utils.go.
//
// The registry uses a singleton pattern with package-level registration
// functions for convenience. Commands register themselves during package
// initialization via init() functions.
type CommandRegistry struct {
	mu        sync.RWMutex
	providers map[string]CommandProvider
}

// Register adds a built-in command provider to the registry.
//
// This function is typically called during package initialization via init():
//
//	func init() {
//	    internal.Register(&AboutCommandProvider{})
//	}
//
// If a provider with the same name is already registered, the new provider
// replaces the existing one. This allows for plugin override functionality
// in the future and makes testing easier.
func Register(provider CommandProvider) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	name := provider.GetName()

	// Note: We allow re-registration for flexibility in testing and future
	// plugin override functionality. In production, each built-in command
	// should only register once during init().
	// The existence check is intentionally not used - we simply overwrite.
	registry.providers[name] = provider
}

// RegisterAll registers all built-in commands with the root command.
//
// This function should be called once during application initialization
// in cmd/root.go init(). It registers all commands that have been added
// to the registry via Register().
//
// This function performs registration in two phases:
// 1. Register all primary commands to the root command
// 2. Register command aliases to their respective parent commands
//
// Custom commands from atmos.yaml are processed AFTER this function
// via processCustomCommands(), allowing custom commands to extend or
// override built-in commands.
//
// Example usage in cmd/root.go:
//
//	func init() {
//	    // Register built-in commands
//	    if err := internal.RegisterAll(RootCmd); err != nil {
//	        log.Error("Failed to register built-in commands", "error", err)
//	    }
//	}
func RegisterAll(root *cobra.Command) error {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	// Phase 1: Register all primary commands.
	for name, provider := range registry.providers {
		cmd := provider.GetCommand()
		if cmd == nil {
			return fmt.Errorf("%w: provider %s", errUtils.ErrCommandNil, name)
		}

		root.AddCommand(cmd)
	}

	// Phase 2: Register command aliases.
	// This must happen after phase 1 to ensure parent commands exist.
	for name, provider := range registry.providers {
		aliases := provider.GetAliases()
		if len(aliases) == 0 {
			continue
		}

		// Get the original command to access subcommands.
		originalCmd := provider.GetCommand()
		if originalCmd == nil {
			return fmt.Errorf("%w: provider %s", errUtils.ErrCommandNil, name)
		}

		for _, alias := range aliases {
			// Find the parent command to add the alias under.
			parentCmd, _, err := root.Find([]string{alias.ParentCommand})
			if err != nil {
				return fmt.Errorf("failed to find parent command %q for alias %q: %w", alias.ParentCommand, alias.Name, err)
			}

			// Determine which command to alias (parent or subcommand).
			var sourceCmd *cobra.Command
			if alias.Subcommand == "" {
				// Alias the parent command itself.
				sourceCmd = originalCmd
			} else {
				// Alias a specific subcommand.
				sourceCmd, _, err = originalCmd.Find([]string{alias.Subcommand})
				if err != nil {
					return fmt.Errorf("failed to find subcommand %q for alias %q: %w", alias.Subcommand, alias.Name, err)
				}
				// Verify we actually found the subcommand and not just the parent.
				if sourceCmd == originalCmd {
					return fmt.Errorf("failed to find subcommand %q for alias %q: subcommand does not exist", alias.Subcommand, alias.Name)
				}
			}

			// Create an alias command that delegates to the source command.
			//
			// Key delegation mechanisms:
			// - Args: Enforces the same argument validation as the source
			// - Run/RunE: Executes the source command's logic (copy whichever is non-nil)
			// - FParseErrWhitelist: Allows the same flag parsing behavior
			// - ValidArgsFunction: Provides the same shell completion
			aliasCmd := &cobra.Command{
				Use:                alias.Name,
				Short:              alias.Short,
				Long:               alias.Long,
				Example:            alias.Example,
				Args:               sourceCmd.Args,
				Run:                sourceCmd.Run,
				RunE:               sourceCmd.RunE,
				FParseErrWhitelist: sourceCmd.FParseErrWhitelist,
				ValidArgsFunction:  sourceCmd.ValidArgsFunction,
			}

			// Share flags with the source command.
			// Using AddFlag shares the same flag instance, which means:
			// 1. Flag values set on the alias are visible to the source RunE
			// 2. Flag validation happens naturally through shared state
			// 3. No need to manually copy flag values between commands
			// This creates true delegation where the alias is just a different
			// path to the same underlying command implementation.
			sourceCmd.Flags().VisitAll(func(flag *pflag.Flag) {
				aliasCmd.Flags().AddFlag(flag)
			})

			// Share flag completion functions.
			// This ensures the alias provides the same shell completion
			// suggestions as the source command for all flags.
			sourceCmd.Flags().VisitAll(func(flag *pflag.Flag) {
				if completionFunc, _ := sourceCmd.GetFlagCompletionFunc(flag.Name); completionFunc != nil {
					_ = aliasCmd.RegisterFlagCompletionFunc(flag.Name, completionFunc)
				}
			})

			// Add the alias command to the parent.
			parentCmd.AddCommand(aliasCmd)
		}
	}

	return nil
}

// GetProvider returns a built-in command provider by name.
//
// This function is primarily used for testing and diagnostics.
// Returns the provider and true if found, nil and false otherwise.
func GetProvider(name string) (CommandProvider, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	provider, ok := registry.providers[name]
	return provider, ok
}

// ListProviders returns all registered providers grouped by category.
//
// This function is useful for generating help text and diagnostics.
// The returned map uses group names as keys and slices of providers as values.
//
// Example output:
//
//	{
//	    "Core Stack Commands": [terraform, helmfile, workflow],
//	    "Stack Introspection": [describe, list, validate],
//	    ...
//	}
func ListProviders() map[string][]CommandProvider {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	grouped := make(map[string][]CommandProvider)

	for _, provider := range registry.providers {
		group := provider.GetGroup()
		grouped[group] = append(grouped[group], provider)
	}

	return grouped
}

// Count returns the number of registered built-in providers.
//
// This function is primarily used for testing and diagnostics.
func Count() int {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	return len(registry.providers)
}

// Reset clears the registry, removing all registered providers.
//
// WARNING: This function is for TESTING ONLY. It should never be called
// in production code. It allows tests to start with a clean registry state.
func Reset() {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.providers = make(map[string]CommandProvider)
}

// GetCompatFlagsForCommand returns compatibility flags for a command provider.
// The providerName should match the top-level command (e.g., "terraform").
// Returns nil if the provider is not found or has no compatibility flags.
//
// This is used during arg preprocessing in Execute() to separate Atmos flags
// from pass-through flags before Cobra parses.
func GetCompatFlagsForCommand(providerName string) map[string]compat.CompatibilityFlag {
	provider, ok := GetProvider(providerName)
	if !ok {
		return nil
	}
	return provider.GetCompatibilityFlags()
}

// commandCompatFlagsRegistry stores compat flags per command (provider/subcommand).
// This replaces the callback-based approach with direct registration.
var commandCompatFlagsRegistry = struct {
	mu    sync.RWMutex
	flags map[string]map[string]map[string]compat.CompatibilityFlag // provider -> subcommand -> flags
}{
	flags: make(map[string]map[string]map[string]compat.CompatibilityFlag),
}

// RegisterCommandCompatFlags registers compat flags for a specific command.
// The providerName is the top-level command (e.g., "terraform").
// The subcommand is the specific command (e.g., "plan", "apply", or "terraform" for the parent).
// Each subcommand registers its own flags in init(), eliminating the need for switch statements.
func RegisterCommandCompatFlags(providerName, subcommand string, flags map[string]compat.CompatibilityFlag) {
	commandCompatFlagsRegistry.mu.Lock()
	defer commandCompatFlagsRegistry.mu.Unlock()

	if commandCompatFlagsRegistry.flags[providerName] == nil {
		commandCompatFlagsRegistry.flags[providerName] = make(map[string]map[string]compat.CompatibilityFlag)
	}
	commandCompatFlagsRegistry.flags[providerName][subcommand] = flags
}

// GetSubcommandCompatFlags returns compatibility flags for a specific subcommand.
// The providerName should match the top-level command (e.g., "terraform").
// The subcommand is the name of the subcommand (e.g., "apply", "plan").
// Returns nil if no flags are registered for the provider/subcommand combination.
func GetSubcommandCompatFlags(providerName, subcommand string) map[string]compat.CompatibilityFlag {
	commandCompatFlagsRegistry.mu.RLock()
	defer commandCompatFlagsRegistry.mu.RUnlock()

	providerFlags, ok := commandCompatFlagsRegistry.flags[providerName]
	if !ok {
		return nil
	}
	return providerFlags[subcommand]
}
