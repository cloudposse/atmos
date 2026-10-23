package cmd

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestCommandDocumentationColor(t *testing.T) {
	_ = NewTestKit(t)
	previous := ui.GetColorProfile()
	t.Cleanup(func() { ui.SetColorProfile(previous) })
	root, _, _ := discoveryTestRoot(t)
	for _, profile := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		ui.SetColorProfile(profile)
		var out bytes.Buffer
		printCommandDocumentation(&out, root)
		assert.Equal(t, "\n  Documentation: "+internal.ResolveCommandMetadata(root).DocumentationURL+"\n", ansi.Strip(out.String()))
		assert.Equal(t, profile != termenv.Ascii, strings.Contains(out.String(), "\x1b["))
	}
}

func TestCommandDocumentationHelpTopics(t *testing.T) {
	_ = NewTestKit(t)
	root, leaf, _ := discoveryTestRoot(t)
	for _, command := range []*cobra.Command{root, leaf} {
		for _, topic := range append([]helpTopic{helpTopicDefault}, supportedHelpTopics...) {
			t.Run(command.Name()+"/"+string(topic), func(t *testing.T) {
				var out bytes.Buffer
				renderer := lipgloss.NewRenderer(&out)
				styles := createHelpStyles(renderer)
				ctx := &helpRenderContext{writer: &out, renderer: renderer, styles: &styles, atmosConfig: &schema.AtmosConfiguration{}}
				printHelpForTopic(ctx, command, helpTopicRequest{topic: topic, valid: true})
				assert.Contains(t, ansi.Strip(out.String()), "Documentation: "+internal.ResolveCommandMetadata(command).DocumentationURL)
				assert.Equal(t, 1, strings.Count(out.String(), "Documentation:"))
				if topic == helpTopicAll {
					assert.Contains(t, out.String(), "--docs")
					assert.Contains(t, out.String(), "--example")
				}
			})
		}
	}
}

// Match Docusaurus's category indexes, local doc IDs, and explicit slugs, rather
// than guessing a URL from the command name. This also catches links to removed
// example folders.
func TestCommandDocumentationCatalog(t *testing.T) {
	routes := map[string]string{"https://atmos.tools": "atmos"}
	expected := make(map[string]string)
	base := "../website/docs"
	require.NoError(t, filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || (filepath.Ext(path) != ".md" && filepath.Ext(path) != ".mdx") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parts := strings.SplitN(string(data), "---", 3)
		if len(parts) != 3 || parts[0] != "" {
			return nil
		}
		var metadata struct {
			Title string `yaml:"title"`
			ID    string `yaml:"id"`
			Slug  string `yaml:"slug"`
		}
		if err := yaml.Unmarshal([]byte(parts[1]), &metadata); err != nil {
			return err
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		url := commandDocumentationRoute(rel, metadata.ID, metadata.Slug)
		routes[url] = metadata.Title
		if strings.HasPrefix(metadata.Title, "atmos ") {
			expected[metadata.Title] = url
		}
		return nil
	}))
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		metadata := internal.GetCommandMetadata(command)
		if metadata.DocumentationURL != "" {
			_, exists := routes[metadata.DocumentationURL]
			assert.True(t, exists, "%s links to missing documentation %s", command.CommandPath(), metadata.DocumentationURL)
			if url := expected[command.CommandPath()]; url != "" {
				assert.Equal(t, url, metadata.DocumentationURL, "%s links to another command's page", command.CommandPath())
			}
		} else if url := expected[command.CommandPath()]; url != "" {
			t.Errorf("Dedicated documentation not registered: %s -> %s", command.CommandPath(), url)
		}
		for _, example := range metadata.Examples {
			assert.DirExists(t, filepath.Join("..", example.Source), command.CommandPath())
			assert.NotEmpty(t, example.Label)
		}
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(RootCmd)
}

// Docusaurus uses the source filename, not the frontmatter ID, when recognizing
// category indexes. An explicit slug takes precedence over that convention.
func commandDocumentationRoute(relativePath, id, slug string) string {
	directory := filepath.Dir(relativePath)
	name := strings.TrimSuffix(filepath.Base(relativePath), filepath.Ext(relativePath))
	var route string
	switch {
	case strings.HasPrefix(slug, "/"):
		route = slug
	case slug != "":
		route = filepath.Join(directory, slug)
	case strings.EqualFold(name, "index"), strings.EqualFold(name, "readme"),
		strings.EqualFold(name, filepath.Base(directory)):
		route = directory
	case id != "":
		route = filepath.Join(directory, id)
	default:
		route = filepath.Join(directory, name)
	}
	if route == "." {
		route = ""
	}
	return strings.TrimRight("https://atmos.tools/"+strings.TrimPrefix(filepath.ToSlash(route), "/"), "/")
}

func TestCommandDocumentationRoute(t *testing.T) {
	tests := []struct {
		name, source, id, slug, expected string
	}{
		{"ordinary page", "cli/commands/terraform/usage.mdx", "", "", "/cli/commands/terraform/usage"},
		{"explicit ID", "cli/commands/terraform/terraform-plan.mdx", "plan", "", "/cli/commands/terraform/plan"},
		{"folder index", "cli/commands/terraform/cache/cache.mdx", "cache", "", "/cli/commands/terraform/cache"},
		{"index page", "cli/commands/auth/user/index.mdx", "user", "", "/cli/commands/auth/user"},
		{"readme page", "cli/commands/example/README.md", "", "", "/cli/commands/example"},
		{"index ignores ID", "cli/commands/Example/example.mdx", "different", "", "/cli/commands/Example"},
		{"relative slug overrides index", "cli/commands/cache/cache.mdx", "cache", "overview", "/cli/commands/cache/overview"},
		{"absolute slug overrides index", "cli/commands/cache/index.mdx", "cache", "/reference/cache", "/reference/cache"},
		{"root index", "index.mdx", "", "", ""},
		{"explicit root slug", "home.mdx", "", "/", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, "https://atmos.tools"+tt.expected, commandDocumentationRoute(tt.source, tt.id, tt.slug))
		})
	}
}
