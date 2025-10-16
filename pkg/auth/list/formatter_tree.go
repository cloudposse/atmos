package list

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"

	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// RenderTree renders providers and identities as a hierarchical tree.
func RenderTree(
	authManager authTypes.AuthManager,
	providers map[string]schema.Provider,
	identities map[string]schema.Identity,
) (string, error) {
	defer perf.Track(nil, "list.RenderTree")()

	// Avoid unused-parameter compile error; pass config to perf if available.
	_ = authManager

	var output strings.Builder

	// Create h1 header style with solid background.
	h1Style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorWhite)).
		Background(lipgloss.Color(theme.ColorBlue)).
		Bold(true).
		Padding(0, 1)

	// Handle empty result.
	if len(providers) == 0 && len(identities) == 0 {
		warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorOrange))
		output.WriteString(warningStyle.Render("No providers or identities configured."))
		output.WriteString(newline)
		return output.String(), nil
	}

	// Title.
	output.WriteString(h1Style.Render("Authentication Configuration"))
	output.WriteString(newline)

	// Build unified tree with providers as roots and identities as children.
	unifiedTree := buildUnifiedTree(providers, identities)
	output.WriteString(unifiedTree)
	output.WriteString(newline)

	return output.String(), nil
}

// buildUnifiedTree builds a unified tree with providers as roots and identities as children.
func buildUnifiedTree(
	providers map[string]schema.Provider,
	identities map[string]schema.Identity,
) string {
	defer perf.Track(nil, "list.buildUnifiedTree")()

	// Create tree styles.
	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(treeBranchColor))

	root := tree.New().
		EnumeratorStyle(branchStyle)

	// Group identities by their direct provider.
	identitiesByProvider := make(map[string][]string)
	standaloneIdentities := make([]string, 0)

	for name, identity := range identities {
		switch {
		case identity.Via != nil && identity.Via.Provider != "":
			providerName := identity.Via.Provider
			identitiesByProvider[providerName] = append(identitiesByProvider[providerName], name)
		case identity.Via != nil && identity.Via.Identity != "":
			// This identity uses another identity - will be handled recursively.
			continue
		default:
			// Standalone identity (e.g., aws/user).
			standaloneIdentities = append(standaloneIdentities, name)
		}
	}

	// Sort provider names.
	providerNames := getSortedProviderNames(providers)

	// Build provider nodes with their direct identities as children.
	for _, providerName := range providerNames {
		provider := providers[providerName]
		providerNode := buildProviderNodeWithIdentities(&provider, providerName, identitiesByProvider[providerName], identities)
		root.Child(providerNode)
	}

	// Add standalone identities (if any).
	if len(standaloneIdentities) > 0 {
		sort.Strings(standaloneIdentities)
		boldStyle := lipgloss.NewStyle().Bold(true)
		standaloneNode := tree.New().
			Root(boldStyle.Render("Standalone Identities")).
			EnumeratorStyle(branchStyle)
		for _, name := range standaloneIdentities {
			identity := identities[name]
			identityNode := buildIdentityNode(&identity, name)
			standaloneNode.Child(identityNode)
		}
		root.Child(standaloneNode)
	}

	return root.String()
}

// formatKeyValue formats a key-value pair with styled key and value.
func formatKeyValue(key, value string) string {
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(treeKeyColor))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(treeValueColor))
	return fmt.Sprintf("%s: %s", keyStyle.Render(key), valueStyle.Render(value))
}

// formatKeyValueURL formats a key-value pair with a URL value using link color.
func formatKeyValueURL(key, value string) string {
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(treeKeyColor))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))
	return fmt.Sprintf("%s: %s", keyStyle.Render(key), valueStyle.Render(value))
}

// buildProviderNodeWithIdentities builds a provider node with its identities as children.
func buildProviderNodeWithIdentities(
	provider *schema.Provider,
	name string,
	identityNames []string,
	allIdentities map[string]schema.Identity,
) *tree.Tree {
	defer perf.Track(nil, "list.buildProviderNodeWithIdentities")()

	// Create branch style.
	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(treeBranchColor))

	// Build provider title.
	title := buildProviderTitle(provider, name)
	providerNode := tree.New().
		Root(title).
		EnumeratorStyle(branchStyle)

	// Add provider attributes.
	addProviderAttributesStyled(providerNode, provider, &branchStyle)

	// Add identities that directly use this provider.
	if len(identityNames) > 0 {
		sort.Strings(identityNames)
		boldStyle := lipgloss.NewStyle().Bold(true)
		identitiesNode := tree.New().
			Root(boldStyle.Render("Identities")).
			EnumeratorStyle(branchStyle)
		for _, identityName := range identityNames {
			identity := allIdentities[identityName]
			identityNode := buildIdentityNodeForProvider(&identity, identityName, allIdentities)
			identitiesNode.Child(identityNode)
		}
		providerNode.Child(identitiesNode)
	}

	return providerNode
}

// buildIdentityNodeForProvider builds an identity node with its child identities (role chains).
func buildIdentityNodeForProvider(
	identity *schema.Identity,
	name string,
	allIdentities map[string]schema.Identity,
) *tree.Tree {
	defer perf.Track(nil, "list.buildIdentityNodeForProvider")()

	// Create branch style.
	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(treeBranchColor))

	// Build identity title.
	title := buildIdentityTitle(identity, name)
	identityNode := tree.New().
		Root(title).
		EnumeratorStyle(branchStyle)

	// Add principal if present.
	if len(identity.Principal) > 0 {
		boldStyle := lipgloss.NewStyle().Bold(true)
		principalNode := tree.New().
			Root(boldStyle.Render("Principal")).
			EnumeratorStyle(branchStyle)
		addMapToTreeStyled(principalNode, identity.Principal, 0, &branchStyle)
		identityNode.Child(principalNode)
	}

	// Find child identities (identities that use this identity via Via.Identity).
	childIdentities := make([]string, 0)
	for childName, childIdentity := range allIdentities {
		if childIdentity.Via != nil && childIdentity.Via.Identity == name {
			childIdentities = append(childIdentities, childName)
		}
	}

	// Add child identities recursively.
	if len(childIdentities) > 0 {
		sort.Strings(childIdentities)
		for _, childName := range childIdentities {
			childIdentity := allIdentities[childName]
			childNode := buildIdentityNodeForProvider(&childIdentity, childName, allIdentities)
			identityNode.Child(childNode)
		}
	}

	return identityNode
}

// buildProvidersTree builds a tree representation of providers.
func buildProvidersTree(providers map[string]schema.Provider) string {
	defer perf.Track(nil, "list.buildProvidersTree")()

	// Sort provider names.
	names := getSortedProviderNames(providers)

	// Build tree nodes.
	root := tree.Root("")
	for _, name := range names {
		provider := providers[name]
		providerNode := buildProviderNode(&provider, name)
		root.Child(providerNode)
	}

	return root.String()
}

// buildProviderNode builds a tree node for a single provider.
func buildProviderNode(provider *schema.Provider, name string) *tree.Tree {
	defer perf.Track(nil, "list.buildProviderNode")()

	// Build provider node title.
	title := buildProviderTitle(provider, name)
	providerNode := tree.New().Root(title)

	// Add provider attributes.
	addProviderAttributes(providerNode, provider)

	return providerNode
}

// buildProviderTitle builds the title string for a provider node.
func buildProviderTitle(provider *schema.Provider, name string) string {
	title := fmt.Sprintf("%s (%s)", name, provider.Kind)
	if provider.Default {
		title += " [DEFAULT]"
	}
	return title
}

// addProviderAttributes adds provider attributes to a node.
func addProviderAttributes(node *tree.Tree, provider *schema.Provider) {
	if provider.Region != "" {
		node.Child(fmt.Sprintf("Region: %s", provider.Region))
	}

	if provider.StartURL != "" {
		node.Child(fmt.Sprintf("Start URL: %s", provider.StartURL))
	} else if provider.URL != "" {
		node.Child(fmt.Sprintf("URL: %s", provider.URL))
	}

	if provider.Username != "" {
		node.Child(fmt.Sprintf("Username: %s", provider.Username))
	}

	if provider.ProviderType != "" {
		node.Child(fmt.Sprintf("Provider Type: %s", provider.ProviderType))
	}

	// Add session config if present.
	if provider.Session != nil && provider.Session.Duration != "" {
		sessionNode := tree.New().Root("Session")
		sessionNode.Child(fmt.Sprintf("Duration: %s", provider.Session.Duration))
		node.Child(sessionNode)
	}
}

// addProviderAttributesStyled adds provider attributes to a node with styling.
func addProviderAttributesStyled(node *tree.Tree, provider *schema.Provider, branchStyle *lipgloss.Style) {
	if provider.Region != "" {
		node.Child(formatKeyValue("Region", provider.Region))
	}

	if provider.StartURL != "" {
		node.Child(formatKeyValueURL("Start URL", provider.StartURL))
	} else if provider.URL != "" {
		node.Child(formatKeyValueURL("URL", provider.URL))
	}

	if provider.Username != "" {
		node.Child(formatKeyValue("Username", provider.Username))
	}

	if provider.ProviderType != "" {
		node.Child(formatKeyValue("Provider Type", provider.ProviderType))
	}

	// Add session config if present.
	if provider.Session != nil && provider.Session.Duration != "" {
		boldStyle := lipgloss.NewStyle().Bold(true)
		sessionNode := tree.New().
			Root(boldStyle.Render("Session")).
			EnumeratorStyle(*branchStyle)
		sessionNode.Child(formatKeyValue("Duration", provider.Session.Duration))
		node.Child(sessionNode)
	}
}

// buildIdentitiesTree builds a tree representation of identities.
func buildIdentitiesTree(identities map[string]schema.Identity) string {
	defer perf.Track(nil, "list.buildIdentitiesTree")()

	// Sort identity names.
	names := getSortedIdentityNames(identities)

	// Build tree nodes.
	root := tree.Root("")
	for _, name := range names {
		identity := identities[name]
		identityNode := buildIdentityNode(&identity, name)
		root.Child(identityNode)
	}

	return root.String()
}

// buildIdentityNode builds a tree node for a single identity.
func buildIdentityNode(identity *schema.Identity, name string) *tree.Tree {
	defer perf.Track(nil, "list.buildIdentityNode")()

	// Build title with flags.
	title := buildIdentityTitle(identity, name)
	identityNode := tree.New().Root(title)

	// Add principal and credentials.
	addIdentityMetadata(identityNode, identity)

	return identityNode
}

// buildIdentityTitle builds the title string for an identity node.
func buildIdentityTitle(identity *schema.Identity, name string) string {
	title := fmt.Sprintf("%s (%s)", name, identity.Kind)
	if identity.Default {
		title += " [DEFAULT]"
	}
	if identity.Alias != "" {
		title += fmt.Sprintf(" [ALIAS: %s]", identity.Alias)
	}
	return title
}

// addIdentityMetadata adds principal and credentials to a node.
func addIdentityMetadata(node *tree.Tree, identity *schema.Identity) {
	// Add principal if present.
	if len(identity.Principal) > 0 {
		principalNode := tree.New().Root("Principal")
		addMapToTree(principalNode, identity.Principal, 0)
		node.Child(principalNode)
	}

	// Add credentials if present (redact sensitive values).
	if len(identity.Credentials) > 0 {
		node.Child("Credentials: [redacted]")
	}
}

// addMapToTree recursively adds map entries to a tree node.
func addMapToTree(node *tree.Tree, data map[string]interface{}, depth int) {
	defer perf.Track(nil, "list.addMapToTree")()

	// Limit depth to prevent extremely deep nesting.
	const maxDepth = 5
	if depth > maxDepth {
		node.Child("...")
		return
	}

	// Sort keys for consistent output.
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := data[key]

		switch v := value.(type) {
		case map[string]interface{}:
			// Nested map: create child node.
			childNode := tree.New().Root(key)
			addMapToTree(childNode, v, depth+1)
			node.Child(childNode)
		case []interface{}:
			// Array: show as string.
			node.Child(fmt.Sprintf("%s: %v", key, v))
		default:
			// Primitive: show as key-value.
			node.Child(fmt.Sprintf("%s: %v", key, v))
		}
	}
}

// addMapToTreeStyled recursively adds map entries to a tree node with styling.
func addMapToTreeStyled(node *tree.Tree, data map[string]interface{}, depth int, branchStyle *lipgloss.Style) {
	defer perf.Track(nil, "list.addMapToTreeStyled")()

	// Limit depth to prevent extremely deep nesting.
	const maxDepth = 5
	if depth > maxDepth {
		node.Child("...")
		return
	}

	// Sort keys for consistent output.
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := data[key]

		switch v := value.(type) {
		case map[string]interface{}:
			// Nested map: create child node with bold key.
			boldStyle := lipgloss.NewStyle().Bold(true)
			childNode := tree.New().
				Root(boldStyle.Render(key)).
				EnumeratorStyle(*branchStyle)
			addMapToTreeStyled(childNode, v, depth+1, branchStyle)
			node.Child(childNode)
		case []interface{}:
			// Array: show as formatted key-value.
			node.Child(formatKeyValue(key, fmt.Sprintf("%v", v)))
		default:
			// Primitive: show as formatted key-value.
			node.Child(formatKeyValue(key, fmt.Sprintf("%v", v)))
		}
	}
}
