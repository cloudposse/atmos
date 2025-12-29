package auth

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	authList "github.com/cloudposse/atmos/pkg/auth/list"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:embed markdown/atmos_auth_list_usage.md
var authListUsageMarkdown string

const (
	providersKey  = "providers"
	identitiesKey = "identities"
	// ListFormatFlagName is the name of the format flag for list command.
	ListFormatFlagName = "format"
)

// listParser handles flags for the list command.
var listParser *flags.StandardParser

// authListCmd lists authentication providers and identities.
var authListCmd = &cobra.Command{
	Use:   "list",
	Short: "List authentication providers and identities",
	Long: `List all configured authentication providers and identities with their relationships and chains.

Supports multiple output formats:
- **tree** (default): hierarchical tree visualization showing authentication chains
- **table**: tabular view with provider and identity details
- **json**/**yaml**: structured data for programmatic access
- **graphviz**: DOT format for Graphviz visualization
- **mermaid**: Mermaid diagram syntax for rendering in compatible tools
- **markdown**: Markdown document with embedded Mermaid diagram`,
	Example:            authListUsageMarkdown,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  cobra.NoFileCompletions,
	RunE:               executeAuthListCommand,
}

func init() {
	defer perf.Track(nil, "auth.list.init")()

	// Create parser with list-specific flags.
	listParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "f", "tree", "Output format: tree, table, json, yaml, graphviz, mermaid, markdown"),
		flags.WithStringFlag("providers", "", "", "Show only providers (optionally filter by name: --providers=aws-sso,okta)"),
		flags.WithStringFlag("identities", "", "", "Show only identities (optionally filter by name: --identities=admin,dev)"),
		flags.WithValidValues("format", "tree", "table", "json", "yaml", "graphviz", "dot", "mermaid", "markdown", "md"),
	)

	// Register flags with the command.
	listParser.RegisterFlags(authListCmd)

	// Bind to Viper for environment variable support.
	if err := listParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register flag completion functions.
	if err := authListCmd.RegisterFlagCompletionFunc("format", listFormatFlagCompletion); err != nil {
		log.Trace("Failed to register format flag completion", "error", err)
	}

	if err := authListCmd.RegisterFlagCompletionFunc("providers", listProvidersFlagCompletion); err != nil {
		log.Trace("Failed to register providers flag completion", "error", err)
	}

	if err := authListCmd.RegisterFlagCompletionFunc("identities", listIdentitiesFlagCompletion); err != nil {
		log.Trace("Failed to register identities flag completion", "error", err)
	}

	// Add to parent command.
	authCmd.AddCommand(authListCmd)
}

// listFormatFlagCompletion provides shell completion for the format flag.
func listFormatFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{"tree", "table", "json", "yaml", "graphviz", "mermaid", "markdown"}, cobra.ShellCompDirectiveNoFileComp
}

// listProvidersFlagCompletion provides shell completion for the providers flag.
func listProvidersFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var providers []string
	if atmosConfig.Auth.Providers != nil {
		for name := range atmosConfig.Auth.Providers {
			providers = append(providers, name)
		}
	}

	sort.Strings(providers)

	return providers, cobra.ShellCompDirectiveNoFileComp
}

// listIdentitiesFlagCompletion provides shell completion for the identities flag.
func listIdentitiesFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var identities []string
	if atmosConfig.Auth.Identities != nil {
		for name := range atmosConfig.Auth.Identities {
			identities = append(identities, name)
		}
	}

	sort.Strings(identities)

	return identities, cobra.ShellCompDirectiveNoFileComp
}

// filterConfig holds parsed filter configuration.
type filterConfig struct {
	showProvidersOnly  bool
	showIdentitiesOnly bool
	providerNames      []string
	identityNames      []string
}

// executeAuthListCommand executes the auth list command.
func executeAuthListCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "auth.executeAuthListCommand")()

	handleHelpRequest(cmd, args)

	// Bind parsed flags to Viper for precedence.
	v := viper.GetViper()
	if err := listParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Parse and validate filters.
	filters, err := parseFilterFlags(cmd)
	if err != nil {
		return err
	}

	// Load auth manager.
	authManager, err := loadAuthManagerForList(cmd, v)
	if err != nil {
		return err
	}

	// Get providers and identities.
	providers := authManager.GetProviders()
	identities := authManager.GetIdentities()

	// Apply filters.
	filteredProviders, filteredIdentities, err := applyFilters(providers, identities, filters)
	if err != nil {
		return err
	}

	// Get output format.
	format := v.GetString(ListFormatFlagName)

	// Route to appropriate formatter.
	output, err := renderOutput(authManager, filteredProviders, filteredIdentities, format)
	if err != nil {
		return err
	}

	// Print output (to stdout for data, stderr for UI messages is handled by formatters).
	fmt.Print(output)
	return nil
}

// parseFilterFlags parses and validates filter flags.
func parseFilterFlags(cmd *cobra.Command) (*filterConfig, error) {
	defer perf.Track(nil, "auth.parseFilterFlags")()

	v := viper.GetViper()
	providersFlag := v.GetString(providersKey)
	identitiesFlag := v.GetString(identitiesKey)

	hasProvidersFlag := cmd.Flags().Changed(providersKey)
	hasIdentitiesFlag := cmd.Flags().Changed(identitiesKey)

	// Validate mutual exclusivity.
	if hasProvidersFlag && hasIdentitiesFlag {
		return nil, errUtils.ErrMutuallyExclusiveFlags
	}

	config := &filterConfig{
		showProvidersOnly:  hasProvidersFlag,
		showIdentitiesOnly: hasIdentitiesFlag,
	}

	// Parse provider names.
	if hasProvidersFlag {
		config.providerNames = parseCommaSeparatedNames(providersFlag)
	}

	// Parse identity names.
	if hasIdentitiesFlag {
		config.identityNames = parseCommaSeparatedNames(identitiesFlag)
	}

	return config, nil
}

// parseCommaSeparatedNames parses a comma-separated string into a slice of trimmed names.
func parseCommaSeparatedNames(input string) []string {
	if input == "" {
		return nil
	}

	names := strings.Split(input, ",")
	result := make([]string, 0, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// applyFilters applies filter configuration to providers and identities.
func applyFilters(
	providers map[string]schema.Provider,
	identities map[string]schema.Identity,
	filters *filterConfig,
) (map[string]schema.Provider, map[string]schema.Identity, error) {
	defer perf.Track(nil, "auth.applyFilters")()

	// Show only providers.
	if filters.showProvidersOnly {
		return filterProviders(providers, filters.providerNames)
	}

	// Show only identities.
	if filters.showIdentitiesOnly {
		return filterIdentities(identities, filters.identityNames)
	}

	// Default: show both.
	return providers, identities, nil
}

// filterProviders filters providers by name and returns empty identities.
func filterProviders(
	providers map[string]schema.Provider,
	names []string,
) (map[string]schema.Provider, map[string]schema.Identity, error) {
	defer perf.Track(nil, "auth.filterProviders")()

	filtered := make(map[string]schema.Provider)
	empty := make(map[string]schema.Identity)

	// If no names specified, return all providers.
	if len(names) == 0 {
		return providers, empty, nil
	}

	// Filter by name.
	for _, name := range names {
		provider, exists := providers[name]
		if !exists {
			return nil, nil, fmt.Errorf("%w: %q", errUtils.ErrProviderNotFound, name)
		}
		filtered[name] = provider
	}

	return filtered, empty, nil
}

// filterIdentities filters identities by name and returns empty providers.
func filterIdentities(
	identities map[string]schema.Identity,
	names []string,
) (map[string]schema.Provider, map[string]schema.Identity, error) {
	defer perf.Track(nil, "auth.filterIdentities")()

	empty := make(map[string]schema.Provider)
	filtered := make(map[string]schema.Identity)

	// If no names specified, return all identities.
	if len(names) == 0 {
		return empty, identities, nil
	}

	// Filter by name.
	for _, name := range names {
		identity, exists := identities[name]
		if !exists {
			return nil, nil, fmt.Errorf("%w: %q", errUtils.ErrIdentityNotFound, name)
		}
		filtered[name] = identity
	}

	return empty, filtered, nil
}

// renderOutput routes to the appropriate formatter based on format.
func renderOutput(
	authManager authTypes.AuthManager,
	providers map[string]schema.Provider,
	identities map[string]schema.Identity,
	format string,
) (string, error) {
	defer perf.Track(nil, "auth.renderOutput")()

	switch format {
	case "table":
		return authList.RenderTable(authManager, providers, identities)
	case "tree":
		return authList.RenderTree(authManager, providers, identities)
	case "json":
		return renderJSON(providers, identities)
	case "yaml":
		return renderYAML(providers, identities)
	case "graphviz", "dot":
		return authList.RenderGraphviz(authManager, providers, identities)
	case "mermaid":
		return authList.RenderMermaid(authManager, providers, identities)
	case "markdown", "md":
		return authList.RenderMarkdown(authManager, providers, identities)
	default:
		return "", fmt.Errorf("%w: invalid format %q (valid formats: table, tree, json, yaml, graphviz, mermaid, markdown)", errUtils.ErrInvalidFlag, format)
	}
}

// renderJSON renders providers and identities as JSON.
func renderJSON(providers map[string]schema.Provider, identities map[string]schema.Identity) (string, error) {
	defer perf.Track(nil, "auth.renderJSON")()

	output := map[string]interface{}{}

	if len(providers) > 0 {
		output[providersKey] = providers
	}

	if len(identities) > 0 {
		output[identitiesKey] = identities
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", fmt.Errorf("%w: failed to marshal JSON: %w", errUtils.ErrParseFile, err)
	}

	return string(data) + "\n", nil
}

// renderYAML renders providers and identities as YAML.
func renderYAML(providers map[string]schema.Provider, identities map[string]schema.Identity) (string, error) {
	defer perf.Track(nil, "auth.renderYAML")()

	output := map[string]interface{}{}

	if len(providers) > 0 {
		output[providersKey] = providers
	}

	if len(identities) > 0 {
		output[identitiesKey] = identities
	}

	data, err := yaml.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("%w: failed to marshal YAML: %w", errUtils.ErrParseFile, err)
	}

	return string(data), nil
}

// loadAuthManagerForList loads the auth manager.
func loadAuthManagerForList(cmd *cobra.Command, v *viper.Viper) (authTypes.AuthManager, error) {
	defer perf.Track(nil, "auth.loadAuthManagerForList")()

	// Parse global flags and build ConfigAndStacksInfo to honor --base-path, --config, --config-path, --profile.
	configAndStacksInfo := BuildConfigAndStacksInfo(cmd, v)

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load atmos config: %w", errUtils.ErrInvalidAuthConfig, err)
	}

	manager, err := CreateAuthManager(&atmosConfig.Auth)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create auth manager: %w", errUtils.ErrInvalidAuthConfig, err)
	}

	return manager, nil
}
