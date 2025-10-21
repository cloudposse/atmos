package cmd

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	authList "github.com/cloudposse/atmos/pkg/auth/list"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:embed markdown/atmos_auth_list_usage.md
var authListUsageMarkdown string

const (
	providersKey  = "providers"
	identitiesKey = "identities"
)

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
	defer perf.Track(nil, "cmd.init.authListCmd")()

	// Format flag.
	authListCmd.Flags().StringP("format", "f", "tree", "Output format: tree, table, json, yaml, graphviz, mermaid, markdown")

	// Filter flags with optional string values.
	authListCmd.Flags().String("providers", "", "Show only providers (optionally filter by name: --providers=aws-sso,okta)")
	authListCmd.Flags().String("identities", "", "Show only identities (optionally filter by name: --identities=admin,dev)")

	// Register flag completion functions.
	if err := authListCmd.RegisterFlagCompletionFunc("format", formatFlagCompletion); err != nil {
		log.Trace("Failed to register format flag completion", "error", err)
	}

	if err := authListCmd.RegisterFlagCompletionFunc("providers", providersFlagCompletion); err != nil {
		log.Trace("Failed to register providers flag completion", "error", err)
	}

	if err := authListCmd.RegisterFlagCompletionFunc("identities", identitiesFlagCompletion); err != nil {
		log.Trace("Failed to register identities flag completion", "error", err)
	}

	authCmd.AddCommand(authListCmd)
}

// formatFlagCompletion provides shell completion for the format flag.
func formatFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{"tree", "table", "json", "yaml", "graphviz", "mermaid", "markdown"}, cobra.ShellCompDirectiveNoFileComp
}

// providersFlagCompletion provides shell completion for the providers flag.
func providersFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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

	return providers, cobra.ShellCompDirectiveNoFileComp
}

// identitiesFlagCompletion provides shell completion for the identities flag.
func identitiesFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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
	defer perf.Track(nil, "cmd.executeAuthListCommand")()

	handleHelpRequest(cmd, args)

	// Parse and validate filters.
	filters, err := parseFilterFlags(cmd)
	if err != nil {
		return err
	}

	// Load auth manager.
	authManager, err := loadAuthManagerForList()
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
	format, _ := cmd.Flags().GetString("format")

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
	defer perf.Track(nil, "cmd.parseFilterFlags")()

	providersFlag, _ := cmd.Flags().GetString(providersKey)
	identitiesFlag, _ := cmd.Flags().GetString(identitiesKey)

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
	defer perf.Track(nil, "cmd.applyFilters")()

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
	defer perf.Track(nil, "cmd.filterProviders")()

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
	defer perf.Track(nil, "cmd.filterIdentities")()

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
	defer perf.Track(nil, "cmd.renderOutput")()

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
	defer perf.Track(nil, "cmd.renderJSON")()

	output := map[string]interface{}{}

	if len(providers) > 0 {
		output[providersKey] = providers
	}

	if len(identities) > 0 {
		output[identitiesKey] = identities
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", errors.Join(errUtils.ErrParseFile, fmt.Errorf("failed to marshal JSON: %w", err))
	}

	return string(data) + "\n", nil
}

// renderYAML renders providers and identities as YAML.
func renderYAML(providers map[string]schema.Provider, identities map[string]schema.Identity) (string, error) {
	defer perf.Track(nil, "cmd.renderYAML")()

	output := map[string]interface{}{}

	if len(providers) > 0 {
		output[providersKey] = providers
	}

	if len(identities) > 0 {
		output[identitiesKey] = identities
	}

	data, err := yaml.Marshal(output)
	if err != nil {
		return "", errors.Join(errUtils.ErrParseFile, fmt.Errorf("failed to marshal YAML: %w", err))
	}

	return string(data), nil
}

// loadAuthManager loads the auth manager (helper from auth_whoami.go).
func loadAuthManagerForList() (authTypes.AuthManager, error) {
	defer perf.Track(nil, "cmd.loadAuthManagerForList")()

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, errors.Join(errUtils.ErrInvalidAuthConfig, fmt.Errorf("failed to load atmos config: %w", err))
	}

	manager, err := createAuthManager(&atmosConfig.Auth)
	if err != nil {
		return nil, errors.Join(errUtils.ErrInvalidAuthConfig, fmt.Errorf("failed to create auth manager: %w", err))
	}

	return manager, nil
}
