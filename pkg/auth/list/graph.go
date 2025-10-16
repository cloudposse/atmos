package list

import (
	"fmt"
	"strings"

	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// escapeGraphvizLabel escapes special characters for Graphviz labels.
func escapeGraphvizLabel(s string) string {
	// Escape backslashes first.
	s = strings.ReplaceAll(s, "\\", "\\\\")
	// Escape quotes.
	s = strings.ReplaceAll(s, "\"", "\\\"")
	// Escape newlines.
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// escapeGraphvizID escapes special characters for Graphviz node IDs.
func escapeGraphvizID(s string) string {
	// For node IDs, we just escape quotes since they're wrapped in quotes.
	return strings.ReplaceAll(s, "\"", "\\\"")
}

// RenderGraphviz renders providers and identities as Graphviz DOT format.
func RenderGraphviz(
	authManager authTypes.AuthManager,
	providers map[string]schema.Provider,
	identities map[string]schema.Identity,
) (string, error) {
	defer perf.Track(nil, "list.RenderGraphviz")()

	// Avoid unused-parameter compile error; pass config to perf if available.
	_ = authManager

	var output strings.Builder

	// Handle empty result.
	if len(providers) == 0 && len(identities) == 0 {
		return "digraph AuthConfig {\n  // No providers or identities configured\n}\n", nil
	}

	output.WriteString("digraph AuthConfig {\n")
	output.WriteString("  rankdir=LR;\n")
	output.WriteString("  node [shape=box, style=rounded];\n")
	output.WriteString(newline)

	// Add provider nodes.
	providerNames := getSortedProviderNames(providers)
	for _, name := range providerNames {
		provider := providers[name]
		escapedName := escapeGraphvizID(name)
		label := fmt.Sprintf("%s\\n(%s)", escapeGraphvizLabel(name), escapeGraphvizLabel(provider.Kind))
		if provider.Default {
			output.WriteString(fmt.Sprintf("  \"%s\" [label=\"%s\", style=\"rounded,filled\", fillcolor=lightblue];\n", escapedName, label))
		} else {
			output.WriteString(fmt.Sprintf("  \"%s\" [label=\"%s\"];\n", escapedName, label))
		}
	}

	output.WriteString(newline)

	// Add identity nodes and edges.
	identityNames := getSortedIdentityNames(identities)
	for _, name := range identityNames {
		identity := identities[name]
		escapedName := escapeGraphvizID(name)
		label := fmt.Sprintf("%s\\n(%s)", escapeGraphvizLabel(name), escapeGraphvizLabel(identity.Kind))
		if identity.Default {
			output.WriteString(fmt.Sprintf("  \"%s\" [label=\"%s\", style=\"rounded,filled\", fillcolor=lightgreen];\n", escapedName, label))
		} else {
			output.WriteString(fmt.Sprintf("  \"%s\" [label=\"%s\"];\n", escapedName, label))
		}

		// Add edges for via relationships.
		if identity.Via != nil {
			if identity.Via.Provider != "" {
				escapedProvider := escapeGraphvizID(identity.Via.Provider)
				output.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\";\n", escapedProvider, escapedName))
			}
			if identity.Via.Identity != "" {
				escapedViaIdentity := escapeGraphvizID(identity.Via.Identity)
				output.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\";\n", escapedViaIdentity, escapedName))
			}
		}
	}

	output.WriteString("}\n")
	return output.String(), nil
}

// escapeMermaidLabel escapes special characters for Mermaid labels.
func escapeMermaidLabel(s string) string {
	// Escape quotes for Mermaid labels.
	s = strings.ReplaceAll(s, "\"", "&quot;")
	// Escape angle brackets.
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// renderMermaidProviders renders provider nodes and returns default provider IDs.
func renderMermaidProviders(output *strings.Builder, providers map[string]schema.Provider) []string {
	var defaultProviders []string
	providerNames := getSortedProviderNames(providers)
	for _, name := range providerNames {
		provider := providers[name]
		label := fmt.Sprintf("%s<br/>%s", escapeMermaidLabel(name), escapeMermaidLabel(provider.Kind))
		nodeID := sanitizeMermaidID(name)
		fmt.Fprintf(output, "  %s[\"%s\"]\n", nodeID, label)
		if provider.Default {
			defaultProviders = append(defaultProviders, nodeID)
		}
	}
	return defaultProviders
}

// renderMermaidIdentities renders identity nodes and returns default identity IDs.
func renderMermaidIdentities(output *strings.Builder, identities map[string]schema.Identity) []string {
	var defaultIdentities []string
	identityNames := getSortedIdentityNames(identities)
	for _, name := range identityNames {
		identity := identities[name]
		label := fmt.Sprintf("%s<br/>%s", escapeMermaidLabel(name), escapeMermaidLabel(identity.Kind))
		nodeID := sanitizeMermaidID(name)
		fmt.Fprintf(output, "  %s[\"%s\"]\n", nodeID, label)
		if identity.Default {
			defaultIdentities = append(defaultIdentities, nodeID)
		}
	}
	return defaultIdentities
}

// renderMermaidEdges renders edges for identity relationships.
func renderMermaidEdges(output *strings.Builder, identities map[string]schema.Identity) {
	identityNames := getSortedIdentityNames(identities)
	for _, name := range identityNames {
		identity := identities[name]
		nodeID := sanitizeMermaidID(name)

		// Add edges for via relationships.
		if identity.Via != nil {
			if identity.Via.Provider != "" {
				fmt.Fprintf(output, "  %s --> %s\n", sanitizeMermaidID(identity.Via.Provider), nodeID)
			}
			if identity.Via.Identity != "" {
				fmt.Fprintf(output, "  %s --> %s\n", sanitizeMermaidID(identity.Via.Identity), nodeID)
			}
		}
	}
}

// applyMermaidClasses applies class directives to nodes.
func applyMermaidClasses(output *strings.Builder, providers map[string]schema.Provider, identities map[string]schema.Identity, defaultProviders, defaultIdentities []string) {
	output.WriteString(newline)
	providerNames := getSortedProviderNames(providers)
	for _, name := range providerNames {
		nodeID := sanitizeMermaidID(name)
		fmt.Fprintf(output, "  class %s provider\n", nodeID)
	}
	for _, nodeID := range defaultProviders {
		fmt.Fprintf(output, "  class %s default\n", nodeID)
	}
	identityNames := getSortedIdentityNames(identities)
	for _, name := range identityNames {
		nodeID := sanitizeMermaidID(name)
		fmt.Fprintf(output, "  class %s identity\n", nodeID)
	}
	for _, nodeID := range defaultIdentities {
		fmt.Fprintf(output, "  class %s default\n", nodeID)
	}
}

// RenderMermaid renders providers and identities as Mermaid diagram.
func RenderMermaid(
	authManager authTypes.AuthManager,
	providers map[string]schema.Provider,
	identities map[string]schema.Identity,
) (string, error) {
	defer perf.Track(nil, "list.RenderMermaid")()

	// Avoid unused-parameter compile error; pass config to perf if available.
	_ = authManager

	var output strings.Builder

	// Handle empty result.
	if len(providers) == 0 && len(identities) == 0 {
		output.WriteString("graph LR\n")
		output.WriteString("  Empty[No providers or identities configured]\n")
		return output.String(), nil
	}

	output.WriteString("graph LR\n")

	// Render all nodes first and track defaults.
	defaultProviders := renderMermaidProviders(&output, providers)
	defaultIdentities := renderMermaidIdentities(&output, identities)

	// Then render all edges.
	renderMermaidEdges(&output, identities)

	// Add class definitions.
	output.WriteString(newline)
	output.WriteString("  classDef provider fill:#e3f2fd,stroke:#1976d2,stroke-width:2px\n")
	output.WriteString("  classDef identity fill:#e8f5e9,stroke:#388e3c,stroke-width:2px\n")
	output.WriteString("  classDef default stroke:#ff9800,stroke-width:3px\n")

	// Apply classes to nodes.
	output.WriteString(newline)
	applyMermaidClasses(&output, providers, identities, defaultProviders, defaultIdentities)

	return output.String(), nil
}

// RenderMarkdown renders providers and identities as Markdown with Mermaid code fence.
func RenderMarkdown(
	authManager authTypes.AuthManager,
	providers map[string]schema.Provider,
	identities map[string]schema.Identity,
) (string, error) {
	defer perf.Track(nil, "list.RenderMarkdown")()

	var output strings.Builder

	output.WriteString("# Authentication Configuration\n\n")

	// Handle empty result.
	if len(providers) == 0 && len(identities) == 0 {
		output.WriteString("No providers or identities configured.\n")
		return output.String(), nil
	}

	output.WriteString("```mermaid\n")
	mermaid, err := RenderMermaid(authManager, providers, identities)
	if err != nil {
		return "", fmt.Errorf("list.RenderMarkdown: mermaid rendering failed: %w", err)
	}
	output.WriteString(mermaid)
	output.WriteString("```\n")

	return output.String(), nil
}

// sanitizeMermaidID sanitizes a string to be a valid Mermaid node ID.
func sanitizeMermaidID(s string) string {
	// Replace characters that are problematic in Mermaid IDs.
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return s
}
