package internal

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandMetadataInheritance(t *testing.T) {
	root := &cobra.Command{Use: "atmos"}
	parent := &cobra.Command{Use: "terraform", Aliases: []string{"tf"}}
	child := &cobra.Command{Use: "plan"}
	root.AddCommand(parent)
	parent.AddCommand(child)
	RegisterCommandMetadata(root, WithDocumentationURL("https://atmos.tools"))
	RegisterCommandMetadata(parent, WithDocumentationURL("https://atmos.tools/terraform"),
		WithExample("examples/simple", "Simple"), WithExample("examples/advanced", "Advanced"))
	RegisterCommandMetadata(child, WithDocumentationURL("https://atmos.tools/terraform/plan"))
	metadata := ResolveCommandMetadata(child)
	assert.Equal(t, "https://atmos.tools/terraform/plan", metadata.DocumentationURL)
	require.Len(t, metadata.Examples, 2)
	assert.Equal(t, "Simple", metadata.Examples[0].Label)
	// Returned metadata is independent of registered values.
	metadata.Examples[0].Label = "changed"
	assert.Equal(t, "Simple", ResolveCommandMetadata(child).Examples[0].Label)
	RegisterCommandMetadata(child, WithExample("examples/plan", "Plan"))
	assert.Equal(t, []CommandExample{{Source: "examples/plan", Label: "Plan"}}, ResolveCommandMetadata(child).Examples)
	alias, _, err := root.Find([]string{"tf", "plan"})
	require.NoError(t, err)
	assert.Equal(t, ResolveCommandMetadata(child), ResolveCommandMetadata(alias))
	assert.Equal(t, "https://atmos.tools", ResolveCommandMetadata(nil).DocumentationURL)
}

func TestCommandMetadataRegistrationAndCrossCommandAlias(t *testing.T) {
	original := registry
	registry = &CommandRegistry{providers: make(map[string]CommandProvider)}
	t.Cleanup(func() { registry = original })
	root := &cobra.Command{Use: "atmos"}
	source := &cobra.Command{Use: "profile"}
	list := &cobra.Command{Use: "list"}
	source.AddCommand(list)
	RegisterCommandMetadata(list, WithDocumentationURL("https://atmos.tools/profile/list"))
	Register(&mockCommandProvider{name: "list", cmd: &cobra.Command{Use: "list"}})
	Register(&mockCommandProvider{name: "profile", cmd: source, aliases: []CommandAlias{{
		ParentCommand: "list", Name: "profiles", Subcommand: "list",
	}}}, WithDocumentationURL("https://atmos.tools/profile"), WithExample("examples/config-profiles", "Profiles"))
	require.NoError(t, RegisterAll(root))
	alias, _, err := root.Find([]string{"list", "profiles"})
	require.NoError(t, err)
	assert.Equal(t, ResolveCommandMetadata(list), ResolveCommandMetadata(alias))
	require.NoError(t, RegisterAll(root))
	assert.Len(t, ResolveCommandMetadata(source).Examples, 1)
}

// A provider is allowed to build a fresh tree each time, as the LSP provider does.
type freshMetadataProvider struct {
	mockCommandProvider
}

func (p *freshMetadataProvider) GetCommand() *cobra.Command {
	return &cobra.Command{Use: p.name}
}

func TestCommandMetadataAliasUsesRegisteredInstance(t *testing.T) {
	original := registry
	registry = &CommandRegistry{providers: make(map[string]CommandProvider)}
	t.Cleanup(func() { registry = original })
	Register(&mockCommandProvider{name: "list", cmd: &cobra.Command{Use: "list"}})
	Register(&freshMetadataProvider{mockCommandProvider: mockCommandProvider{
		name: "example", aliases: []CommandAlias{{ParentCommand: "list", Name: "example"}},
	}}, WithDocumentationURL("https://atmos.tools/example"), WithExample("examples/demo", "Demo"))
	root := &cobra.Command{Use: "atmos"}
	require.NoError(t, RegisterAll(root))
	source, _, err := root.Find([]string{"example"})
	require.NoError(t, err)
	alias, _, err := root.Find([]string{"list", "example"})
	require.NoError(t, err)
	assert.Equal(t, ResolveCommandMetadata(source), ResolveCommandMetadata(alias))
}
