package internal

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/perf"
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
	options   map[string][]CommandOption
	// aliasOptions holds metadata options for aliases, keyed by "<parent> <alias>".
	aliasOptions map[string][]CommandOption
}

// Register adds a built-in command provider to the registry.
//
// This function is typically called during package initialization via init():
//
//	func init() {
//	    internal.Register(&AboutCommandProvider{})
//	}
//
// Re-registering the same provider instance is idempotent. Conflicting startup
// registrations panic; dynamic registration can use TryRegister and handle the error.
func Register(provider CommandProvider, options ...CommandOption) {
	if err := TryRegister(provider, options...); err != nil {
		panic(err)
	}
}

// TryRegister rejects collisions before changing the built-in command registry.
func TryRegister(provider CommandProvider, options ...CommandOption) error {
	defer perf.Track(nil, "internal.TryRegister")()
	if provider == nil {
		return errUtils.ErrCommandNil
	}
	value := reflect.ValueOf(provider)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return errUtils.ErrCommandNil
	}
	name := provider.GetName()
	if name == "" {
		return errUtils.ErrCommandNil
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if existing, found := registry.providers[name]; found {
		if value.Type().Comparable() && reflect.TypeOf(existing) == value.Type() && existing == provider {
			return nil
		}
		return fmt.Errorf("%w: %s", errUtils.ErrDuplicateCommandRegistration, name)
	}
	registry.providers[name] = provider
	if registry.options == nil {
		registry.options = make(map[string][]CommandOption)
	}
	registry.options[name] = options
	return nil
}

// RegisterAliasMetadata overrides the metadata an alias inherits from its source
// command, for example to link an alias to its own documentation page. The alias
// is identified by its parent command and name, as declared in CommandAlias.
func RegisterAliasMetadata(parentCommand, name string, options ...CommandOption) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if registry.aliasOptions == nil {
		registry.aliasOptions = make(map[string][]CommandOption)
	}
	key := parentCommand + " " + name
	registry.aliasOptions[key] = append(registry.aliasOptions[key], options...)
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
	commands := make(map[string]*cobra.Command, len(registry.providers))

	// Phase 1: Register all primary commands.
	for name, provider := range registry.providers {
		cmd := provider.GetCommand()
		if cmd == nil {
			return fmt.Errorf("%w: provider %s", errUtils.ErrCommandNil, name)
		}
		// Start from explicit command metadata on each registration. Keep provider
		// options separate so repeated RegisterAll calls do not append examples.
		metadata := GetCommandMetadata(cmd)
		for _, option := range registry.options[name] {
			option(&metadata)
		}
		// Deduplicate repeat registrations of the same provider's examples.
		metadata.Examples = uniqueExamples(metadata.Examples)
		setCommandMetadata(cmd, metadata)

		// Mark experimental commands with an annotation for later rendering.
		// The actual styled badge is added in formatCommandLine during help rendering
		// when the color profile is available.
		if provider.IsExperimental() {
			markCommandExperimental(cmd)
		}

		root.AddCommand(cmd)
		commands[name] = cmd

		// Apply default NoArgs validator for commands that:
		// 1. Don't already have Args set (explicit validation)
		// 2. Don't have a PositionalArgsBuilder registered (structured args)
		// This prevents commands from silently ignoring unexpected arguments.
		// Commands that need positional args must explicitly declare them.
		if cmd.Args == nil && provider.GetPositionalArgsBuilder() == nil {
			cmd.Args = cobra.NoArgs
		}
	}

	// Phase 2: Register command aliases.
	// This must happen after phase 1 to ensure parent commands exist.
	for name, provider := range registry.providers {
		aliases := provider.GetAliases()
		if len(aliases) == 0 {
			continue
		}

		// Alias the registered instance, including its metadata. Providers may
		// construct a fresh command on each GetCommand call.
		originalCmd := commands[name]

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
				Annotations:        map[string]string{"policy-command": strings.TrimPrefix(sourceCmd.CommandPath(), root.Name()+" ")},
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
			// Aliases inherit their source's metadata unless an alias has its own page.
			aliasMetadata := ResolveCommandMetadata(sourceCmd)
			for _, option := range registry.aliasOptions[alias.ParentCommand+" "+alias.Name] {
				option(&aliasMetadata)
			}
			setCommandMetadata(aliasCmd, aliasMetadata)

			// Share flags with the source command.
			// Using AddFlag shares the same flag instance, which means:
			// 1. Flag values set on the alias are visible to the source RunE
			// 2. Flag validation happens naturally through shared state
			// 3. No need to manually copy flag values between commands
			// This creates true delegation where the alias is just a different
			// path to the same underlying command implementation.
			shareAliasFlags(aliasCmd, sourceCmd, root)

			// Add the alias command to the parent.
			parentCmd.AddCommand(aliasCmd)
		}
	}

	return nil
}

// shareAliasFlags preserves source-local and ancestor-persistent flags, including
// their completions. Root flags remain inherited from the alias's new parent.
func shareAliasFlags(aliasCmd, sourceCmd, root *cobra.Command) {
	share := func(owner *cobra.Command, flagSet *pflag.FlagSet) {
		flagSet.VisitAll(func(flag *pflag.Flag) {
			if aliasCmd.Flags().Lookup(flag.Name) != nil {
				return
			}
			aliasCmd.Flags().AddFlag(flag)
			if completionFunc, _ := owner.GetFlagCompletionFunc(flag.Name); completionFunc != nil {
				_ = aliasCmd.RegisterFlagCompletionFunc(flag.Name, completionFunc)
			}
		})
	}
	share(sourceCmd, sourceCmd.LocalFlags())
	for parent := sourceCmd.Parent(); parent != nil && parent != root; parent = parent.Parent() {
		share(parent, parent.PersistentFlags())
	}
}

// markCommandExperimental sets the experimental annotation on a command and all its subcommands.
func markCommandExperimental(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations["experimental"] = "true"

	// Recursively mark all subcommands.
	for _, sub := range cmd.Commands() {
		markCommandExperimental(sub)
	}
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

// IsCommandExperimental returns true if the named command is experimental.
// Returns false if the command is not found or is not experimental.
func IsCommandExperimental(name string) bool {
	provider, ok := GetProvider(name)
	if !ok {
		return false
	}
	return provider.IsExperimental()
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
	registry.options = make(map[string][]CommandOption)
	registry.aliasOptions = make(map[string][]CommandOption)
}

// GetCompatFlagsForCommand returns compatibility flags for a command provider.
// The providerName should match the top-level command (e.g., "terraform").
// Returns nil if the provider is not found or has no compatibility flags.
//
// This is used during arg preprocessing in Execute() to separate Atmos flags
// from pass-through flags before Cobra parses.
func GetCompatFlagsForCommand(providerName string) map[string]compat.CompatibilityFlag {
	defer perf.Track(nil, "internal.GetCompatFlagsForCommand")()

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
	defer perf.Track(nil, "internal.RegisterCommandCompatFlags")()

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
	defer perf.Track(nil, "internal.GetSubcommandCompatFlags")()

	commandCompatFlagsRegistry.mu.RLock()
	defer commandCompatFlagsRegistry.mu.RUnlock()

	providerFlags, ok := commandCompatFlagsRegistry.flags[providerName]
	if !ok {
		return nil
	}
	return providerFlags[subcommand]
}

// commandFlagRegistries stores FlagRegistry instances per command provider.
// This allows preprocessCompatibilityFlags to use PreprocessNoOptDefValArgs
// from the registered flag registries instead of hardcoded flag maps.
var commandFlagRegistries = struct {
	mu         sync.RWMutex
	registries map[string]*flags.FlagRegistry // provider name -> flag registry
}{
	registries: make(map[string]*flags.FlagRegistry),
}

// RegisterCommandFlagRegistry registers a FlagRegistry for a command provider.
// The providerName is the top-level command (e.g., "terraform").
// This enables preprocessCompatibilityFlags to use the registry's
// PreprocessNoOptDefValArgs for normalizing NoOptDefVal flags.
func RegisterCommandFlagRegistry(providerName string, registry *flags.FlagRegistry) {
	defer perf.Track(nil, "internal.RegisterCommandFlagRegistry")()

	commandFlagRegistries.mu.Lock()
	defer commandFlagRegistries.mu.Unlock()

	commandFlagRegistries.registries[providerName] = registry
}

// GetCommandFlagRegistry returns the FlagRegistry for a command provider.
// Returns nil if no registry is registered for the provider.
func GetCommandFlagRegistry(providerName string) *flags.FlagRegistry {
	defer perf.Track(nil, "internal.GetCommandFlagRegistry")()

	commandFlagRegistries.mu.RLock()
	defer commandFlagRegistries.mu.RUnlock()

	return commandFlagRegistries.registries[providerName]
}
