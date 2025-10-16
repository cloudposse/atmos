package list

import (
	"fmt"
	"strings"

	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// RenderGraphviz renders providers and identities as Graphviz DOT format.
func RenderGraphviz(
	authManager authTypes.AuthManager,
	providers map[string]schema.Provider,
	identities map[string]schema.Identity,
) (string, error) {
	defer perf.Track(nil, "list.RenderGraphviz")()

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
		label := fmt.Sprintf("%s\\n(%s)", name, provider.Kind)
		if provider.Default {
			output.WriteString(fmt.Sprintf("  \"%s\" [label=\"%s\", style=\"rounded,filled\", fillcolor=lightblue];\n", name, label))
		} else {
			output.WriteString(fmt.Sprintf("  \"%s\" [label=\"%s\"];\n", name, label))
		}
	}

	output.WriteString(newline)

	// Add identity nodes and edges.
	identityNames := getSortedIdentityNames(identities)
	for _, name := range identityNames {
		identity := identities[name]
		label := fmt.Sprintf("%s\\n(%s)", name, identity.Kind)
		if identity.Default {
			output.WriteString(fmt.Sprintf("  \"%s\" [label=\"%s\", style=\"rounded,filled\", fillcolor=lightgreen];\n", name, label))
		} else {
			output.WriteString(fmt.Sprintf("  \"%s\" [label=\"%s\"];\n", name, label))
		}

		// Add edges for via relationships.
		if identity.Via != nil {
			if identity.Via.Provider != "" {
				output.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\";\n", identity.Via.Provider, name))
			}
			if identity.Via.Identity != "" {
				output.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\";\n", identity.Via.Identity, name))
			}
		}
	}

	output.WriteString("}\n")
	return output.String(), nil
}

// RenderMermaid renders providers and identities as Mermaid diagram.
func RenderMermaid(
	authManager authTypes.AuthManager,
	providers map[string]schema.Provider,
	identities map[string]schema.Identity,
) (string, error) {
	defer perf.Track(nil, "list.RenderMermaid")()

	var output strings.Builder

	// Handle empty result.
	if len(providers) == 0 && len(identities) == 0 {
		output.WriteString("graph LR\n")
		output.WriteString("  Empty[No providers or identities configured]\n")
		return output.String(), nil
	}

	output.WriteString("graph LR\n")

	// Add provider nodes.
	providerNames := getSortedProviderNames(providers)
	for _, name := range providerNames {
		provider := providers[name]
		label := fmt.Sprintf("%s<br/>%s", name, provider.Kind)
		if provider.Default {
			output.WriteString(fmt.Sprintf("  %s[\"%s\"]:::provider:::default\n", sanitizeMermaidID(name), label))
		} else {
			output.WriteString(fmt.Sprintf("  %s[\"%s\"]:::provider\n", sanitizeMermaidID(name), label))
		}
	}

	// Add identity nodes and edges.
	identityNames := getSortedIdentityNames(identities)
	for _, name := range identityNames {
		identity := identities[name]
		label := fmt.Sprintf("%s<br/>%s", name, identity.Kind)
		if identity.Default {
			output.WriteString(fmt.Sprintf("  %s[\"%s\"]:::identity:::default\n", sanitizeMermaidID(name), label))
		} else {
			output.WriteString(fmt.Sprintf("  %s[\"%s\"]:::identity\n", sanitizeMermaidID(name), label))
		}

		// Add edges for via relationships.
		if identity.Via != nil {
			if identity.Via.Provider != "" {
				output.WriteString(fmt.Sprintf("  %s --> %s\n", sanitizeMermaidID(identity.Via.Provider), sanitizeMermaidID(name)))
			}
			if identity.Via.Identity != "" {
				output.WriteString(fmt.Sprintf("  %s --> %s\n", sanitizeMermaidID(identity.Via.Identity), sanitizeMermaidID(name)))
			}
		}
	}

	// Add styles.
	output.WriteString(newline)
	output.WriteString("  classDef provider fill:#e3f2fd,stroke:#1976d2,stroke-width:2px\n")
	output.WriteString("  classDef identity fill:#e8f5e9,stroke:#388e3c,stroke-width:2px\n")
	output.WriteString("  classDef default stroke:#ff9800,stroke-width:3px\n")

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
		return "", err
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
