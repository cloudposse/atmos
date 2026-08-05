package atmos

import (
	"context"
	"fmt"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Parameter name for filtering auth list output to specific provider names.
	paramProviders = "providers"
	// Parameter name for filtering auth list output to specific identity names.
	paramIdentities = "identities"
)

// AuthListTool lists configured Atmos authentication providers and identities.
type AuthListTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewAuthListTool creates a new auth list tool.
func NewAuthListTool(atmosConfig *schema.AtmosConfiguration) *AuthListTool {
	return &AuthListTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *AuthListTool) Name() string {
	return "atmos_auth_list"
}

// Description returns the tool description.
func (t *AuthListTool) Description() string {
	return "List configured Atmos authentication providers and identities, and how identities chain to " +
		"providers. Read-only introspection of auth configuration; never authenticates or returns credential material."
}

// Parameters returns the tool parameters.
func (t *AuthListTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramProviders,
			Description: "Comma-separated provider names to show (omit to show all providers and identities).",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramIdentities,
			Description: "Comma-separated identity names to show (omit to show all providers and identities).",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute runs the tool.
func (t *AuthListTool) Execute(_ context.Context, params map[string]interface{}) (*tools.Result, error) {
	if t.atmosConfig == nil {
		err := fmt.Errorf("%w: atmos configuration is not loaded", errUtils.ErrAIInvalidConfiguration)
		return &tools.Result{Success: false, Error: err}, err
	}

	providersFilter, _ := params[paramProviders].(string)
	identitiesFilter, _ := params[paramIdentities].(string)
	if providersFilter != "" && identitiesFilter != "" {
		err := errUtils.ErrMutuallyExclusiveFlags
		return &tools.Result{Success: false, Error: err}, err
	}

	authManager, err := auth.NewDefaultManager(&t.atmosConfig.Auth, t.atmosConfig.CliConfigPath)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	providers := authManager.GetProviders()
	identities := authManager.GetIdentities()

	if providersFilter != "" {
		providers, err = filterAuthProviders(providers, parseNameList(providersFilter))
		if err != nil {
			return &tools.Result{Success: false, Error: err}, err
		}
		identities = map[string]schema.Identity{}
	} else if identitiesFilter != "" {
		identities, err = filterAuthIdentities(identities, parseNameList(identitiesFilter))
		if err != nil {
			return &tools.Result{Success: false, Error: err}, err
		}
		providers = map[string]schema.Provider{}
	}

	return buildAuthListResult(providers, identities), nil
}

// parseNameList splits a comma-separated list into trimmed, non-empty names.
func parseNameList(input string) []string {
	if input == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	names := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names
}

// filterAuthProviders narrows providers to the given names, or returns all when names is empty.
func filterAuthProviders(providers map[string]schema.Provider, names []string) (map[string]schema.Provider, error) {
	if len(names) == 0 {
		return providers, nil
	}
	filtered := make(map[string]schema.Provider, len(names))
	for _, name := range names {
		p, ok := providers[name]
		if !ok {
			return nil, fmt.Errorf("%w: %q", errUtils.ErrProviderNotFound, name)
		}
		filtered[name] = p
	}
	return filtered, nil
}

// filterAuthIdentities narrows identities to the given names, or returns all when names is empty.
func filterAuthIdentities(identities map[string]schema.Identity, names []string) (map[string]schema.Identity, error) {
	if len(names) == 0 {
		return identities, nil
	}
	filtered := make(map[string]schema.Identity, len(names))
	for _, name := range names {
		id, ok := identities[name]
		if !ok {
			return nil, fmt.Errorf("%w: %q", errUtils.ErrIdentityNotFound, name)
		}
		filtered[name] = id
	}
	return filtered, nil
}

// authProviderSummary is a redacted, serialization-safe view of a provider. Sensitive fields
// (Password, Spec) are intentionally omitted so this tool never surfaces credential material.
type authProviderSummary struct {
	Kind     string `yaml:"kind" json:"kind"`
	Region   string `yaml:"region,omitempty" json:"region,omitempty"`
	StartURL string `yaml:"start_url,omitempty" json:"start_url,omitempty"`
	URL      string `yaml:"url,omitempty" json:"url,omitempty"`
	Default  bool   `yaml:"default,omitempty" json:"default,omitempty"`
}

// authIdentitySummary is a redacted, serialization-safe view of an identity. Sensitive fields
// (Credentials, Spec) are intentionally omitted so this tool never surfaces credential material.
type authIdentitySummary struct {
	Kind        string `yaml:"kind" json:"kind"`
	Provider    string `yaml:"provider,omitempty" json:"provider,omitempty"`
	ViaProvider string `yaml:"via_provider,omitempty" json:"via_provider,omitempty"`
	ViaIdentity string `yaml:"via_identity,omitempty" json:"via_identity,omitempty"`
	Default     bool   `yaml:"default,omitempty" json:"default,omitempty"`
	Alias       string `yaml:"alias,omitempty" json:"alias,omitempty"`
}

// summarizeAuthProviders builds redacted provider summaries and their sorted names.
func summarizeAuthProviders(providers map[string]schema.Provider) (map[string]authProviderSummary, []string) {
	summaries := make(map[string]authProviderSummary, len(providers))
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
		p := providers[name]
		summaries[name] = authProviderSummary{
			Kind:     p.Kind,
			Region:   p.Region,
			StartURL: p.StartURL,
			URL:      p.URL,
			Default:  p.Default,
		}
	}
	sort.Strings(names)
	return summaries, names
}

// summarizeAuthIdentities builds redacted identity summaries and their sorted names.
func summarizeAuthIdentities(identities map[string]schema.Identity) (map[string]authIdentitySummary, []string) {
	summaries := make(map[string]authIdentitySummary, len(identities))
	names := make([]string, 0, len(identities))
	for name := range identities {
		names = append(names, name)
		id := identities[name]
		summary := authIdentitySummary{
			Kind:     id.Kind,
			Provider: id.Provider,
			Default:  id.Default,
			Alias:    id.Alias,
		}
		if id.Via != nil {
			summary.ViaProvider = id.Via.Provider
			summary.ViaIdentity = id.Via.Identity
		}
		summaries[name] = summary
	}
	sort.Strings(names)
	return summaries, names
}

// renderAuthListOutput builds the human-readable listing for providers and identities.
func renderAuthListOutput(
	providerNames []string, providerSummaries map[string]authProviderSummary,
	identityNames []string, identitySummaries map[string]authIdentitySummary,
) string {
	var output strings.Builder
	fmt.Fprintf(&output, "Providers (%d):\n", len(providerNames))
	for _, name := range providerNames {
		p := providerSummaries[name]
		defaultTag := ""
		if p.Default {
			defaultTag = " (default)"
		}
		fmt.Fprintf(&output, "  - %s [%s]%s\n", name, p.Kind, defaultTag)
	}

	fmt.Fprintf(&output, "\nIdentities (%d):\n", len(identityNames))
	for _, name := range identityNames {
		id := identitySummaries[name]
		defaultTag := ""
		if id.Default {
			defaultTag = " (default)"
		}
		via := id.ViaProvider
		if via == "" {
			via = id.Provider
		}
		fmt.Fprintf(&output, "  - %s [%s] via %s%s\n", name, id.Kind, via, defaultTag)
	}

	return output.String()
}

// buildAuthListResult formats providers and identities into a tools.Result.
func buildAuthListResult(providers map[string]schema.Provider, identities map[string]schema.Identity) *tools.Result {
	providerSummaries, providerNames := summarizeAuthProviders(providers)
	identitySummaries, identityNames := summarizeAuthIdentities(identities)

	return &tools.Result{
		Success: true,
		Output:  renderAuthListOutput(providerNames, providerSummaries, identityNames, identitySummaries),
		Data: map[string]interface{}{
			"providers":  providerSummaries,
			"identities": identitySummaries,
		},
	}
}

// RequiresPermission returns true if this tool needs permission.
func (t *AuthListTool) RequiresPermission() bool {
	return false // Read-only operation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *AuthListTool) IsRestricted() bool {
	return false
}
